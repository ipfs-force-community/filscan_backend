package evm_transfer_task

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/biz/browser"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"strings"
)

// NewEVMTransferTask 构造 EVM 转账任务。
//
// 默认行为（不加任何开关）与历史上线版本逐字一致：每 120 高度重算一次 1 小时累计快照。
// 离线回放需要跳过这段重算时，链式调用 WithSkipAccStats(true)（见其注释）。
func NewEVMTransferTask(repo repository.EvmTransferRepo) *EVMTransferTask {
	return &EVMTransferTask{repo: repo}
}

var _ syncer.Task = (*EVMTransferTask)(nil)

type EVMTransferTask struct {
	repo repository.EvmTransferRepo

	// skipAccStats 跳过每 120 高度的「1 小时累计快照」重算。
	//
	// 为什么可以跳（离线回放为什么要这个开关）：
	// HandlerEvmTransferStats 会重算一份「1 小时累计快照」—— 其 SQL 要扫 fevm.evm_transfers
	// 全表（线上约 1049 万行）再叠窗口函数排序，排序溢出 403MB 临时文件，实测单次 35 秒；
	// 而回放区间内这些「缺口边界」快照**一行都不会被接口读到** —— 接口只读 max(epoch) 那一条，
	// 也就是链头那份快照，它由实时同步器在追链头时维护；回放补的是历史缺口，其 120 倍高度的
	// 快照既不完整（缺口边上的一段数据本来就是缺的）也不被任何查询命中。
	// 于是离线回放期间跳过这段纯耗时计算是安全的、且不改变任何被读取的数据。
	//
	// 默认 false ⇒ 实时同步器（injector/syncer_manager.go）与既有单测完全不受影响。
	skipAccStats bool
}

// WithSkipAccStats 链式开关：跳过每 120 高度的 1 小时累计快照重算（离线回放专用）。
//
// 只在 `--skip-acc-stats` 打开时置 true；默认 false ⇒ 行为与改动前完全一致。
// 返回同一指针，便于在 NewEVMTransferTask(...) 后直接链式调用。
func (e *EVMTransferTask) WithSkipAccStats(skip bool) *EVMTransferTask {
	e.skipAccStats = skip
	return e
}

// shouldCalcAccStats 判定给定高度是否需要重算「1 小时累计快照」。
//
// 语义与改动前逐字一致：原条件是 `len(traces) != 0 && traces[0].Epoch%120 == 0`，
// 而 Exec 在进入本判定之前已对 len(traces) == 0 提前 return，故 `len(traces) != 0` 那半恒真，
// 这里只保留 `epoch%120 == 0`。抽成独立方法只为让「跳过开关」可被单测直接钉住，
// **不改变 Exec 的任何可观察行为**。
func (e EVMTransferTask) shouldCalcAccStats(epoch int64) bool {
	return !e.skipAccStats && epoch%120 == 0
}

func (e EVMTransferTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (e EVMTransferTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = e.repo.DeleteEvmTransfers(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = e.repo.DeleteEvmTransferStats(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (e EVMTransferTask) Name() string {
	return "evm-transfer-task"
}

// isEvmTransferTrace 判定该 trace 是否要作为一条 EVM 转账处理。
//
// 判据顺序是关键：**先判 trace / trace.Detail 是否为 nil，再取 Detail 的字段**。
// 原实现在 nil 判断之前就执行 strings.Split(trace.Detail.Actor, "/") ⇒ 只要聚合器返回的
// trace 文档里缺 Detail（或 traces 里夹了空元素），execTaskOrCalculator 的 recover 就会把
// 该高度变成「recover error: runtime error: invalid memory address or nil pointer dereference」
// 而整高度失败并原地重试 —— 一处缺失字段即可再次造成静默停摆。
// 判定语义与原实现完全一致：调用方为 evm 且方法为 InvokeContract 的区块消息才算。
func isEvmTransferTrace(trace *londobell.TraceMessage) bool {
	if trace == nil || trace.Detail == nil {
		return false
	}
	if !trace.IsBlock || trace.Detail.Method != "InvokeContract" {
		return false
	}
	actorTypeSplit := strings.Split(trace.Detail.Actor, "/")
	return len(actorTypeSplit) != 0 && actorTypeSplit[len(actorTypeSplit)-1] == "evm"
}

func (e EVMTransferTask) Exec(ctx *syncer.Context) (err error) {

	ctx.Debugf("开始同步...")
	if ctx.Empty() {
		return
	}

	val, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	traces := val.([]*londobell.TraceMessage)

	if len(traces) == 0 {
		ctx.Debugf("traces is empty")
		return
	}

	var evmTransfers []*po.EvmTransfer
	var evmTransferStats1h []*po.EvmTransferStat

	for _, trace := range traces {
		if !isEvmTransferTrace(trace) {
			continue
		}

		var actor *londobell.ActorState
		actor, err = ctx.Adapter().Actor(ctx.Context(), trace.To, nil)
		if err != nil {
			return
		}

		var gasCost decimal.Decimal
		if trace.GasCost != nil {
			gasCost = trace.GasCost.TotalCost
		}
		var exitCode *int
		if trace.MsgRct != nil {
			exitCode = &trace.MsgRct.ExitCode
		}
		var cid string
		if trace.SignedCid != nil && *trace.SignedCid != "" {
			cid = *trace.SignedCid
		} else {
			cid = trace.Cid
		}

		evmTransfers = append(evmTransfers, &po.EvmTransfer{
			Epoch:        trace.Epoch,
			MessageCid:   cid,
			ActorID:      actor.ActorID,
			ActorAddress: actor.DelegatedAddr,
			UserAddress:  trace.From.Address(),
			Balance:      actor.Balance,
			GasCost:      gasCost,
			Value:        trace.Value,
			ExitCode:     exitCode,
			MethodName:   trace.Detail.Method,
		})
	}

	if traces[0].Epoch%120 == 0 && e.skipAccStats {
		// 只在真正的边界高度打日志（每 120 高度一次），避免回放百万高度时刷日志
		ctx.Infof("高度 %d: 已跳过 1 小时累计快照重算（--skip-acc-stats）", traces[0].Epoch)
	} else if e.shouldCalcAccStats(traces[0].Epoch) {
		evmTransferStats1h, err = e.HandlerEvmTransferStats(ctx.Context(), chain.Epoch(traces[0].Epoch))
		if err != nil {
			return
		}
	}

	ctx.Debugf("开始保存了, evmTransfers: %d, evmTransferStats1h: %d", len(evmTransfers), len(evmTransferStats1h))

	if evmTransfers != nil {
		err = e.SaveEvmTransfers(ctx.Context(), evmTransfers)
		if err != nil {
			return
		}
	}

	if evmTransferStats1h != nil {
		err = e.SaveEvmTransferStats(ctx.Context(), evmTransferStats1h)
		if err != nil {
			return
		}
	}

	return
}

func (e EVMTransferTask) SaveEvmTransfers(ctx context.Context, evmActors []*po.EvmTransfer) (err error) {
	err = e.repo.SaveEvmTransfers(ctx, evmActors)
	if err != nil {
		return
	}
	return
}

func (e EVMTransferTask) HandlerEvmTransferStats(ctx context.Context, epoch chain.Epoch) (evmActorStats1h []*po.EvmTransferStat, err error) {
	// 处理 interval 变化
	evmActorStats1h, err = e.getAccEvmTransferStats(ctx, epoch, "1h")
	if err != nil {
		return
	}

	return
}

func (e EVMTransferTask) getAccEvmTransferStats(ctx context.Context, epoch chain.Epoch, interval string) (evmTransferStats []*po.EvmTransferStat, err error) {
	evmTransfers, err := e.repo.GetEvmTransferStats(ctx, epoch)
	if err != nil {
		return
	}
	for _, evm := range evmTransfers {
		var ethAddress string
		ethAddress, err = browser.TransferToETHAddress(evm.ActorAddress)
		if err != nil {
			return
		}
		evmTransferStats = append(evmTransferStats, &po.EvmTransferStat{
			Epoch:            epoch.Int64(),
			ActorID:          evm.ActorID,
			Interval:         interval,
			AccTransferCount: evm.AccTransferCount,
			AccUserCount:     evm.AccUserCount,
			AccGasCost:       evm.AccGasCost,
			ActorBalance:     evm.ActorBalance,
			ActorAddress:     evm.ActorAddress,
			ContractAddress:  ethAddress,
			ContractName:     evm.ContractName,
		})
	}

	return
}

func (e EVMTransferTask) SaveEvmTransferStats(ctx context.Context, transferStats []*po.EvmTransferStat) (err error) {

	err = e.repo.SaveEvmTransferStats(ctx, transferStats)
	if err != nil {
		return
	}

	return
}
