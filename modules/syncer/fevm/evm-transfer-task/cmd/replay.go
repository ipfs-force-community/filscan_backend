package evmtransfercmd

import (
	"fmt"
	"strings"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	evm_transfer_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

const (
	// 未显式配置时的回退值（与 syncer.Init 的内置默认一致）
	defaultEpochsChunk     int64 = 3
	defaultEpochsThreshold int64 = 20
	defaultErrorWait             = 15 * time.Second
)

// replayTaskName 回放任务名：取自任务自身，避免报告文案与任务名漂移
var replayTaskName = evm_transfer_task.EVMTransferTask{}.Name()

// Range 离线回放的高度区间（左闭右闭，与 syncer 的 InitEpoch / StopEpoch 语义一致：
// 高度 = StopEpoch 的那一批跑完即退出，不会多跑一个高度）
type Range struct {
	From chain.Epoch
	To   chain.Epoch
}

// Count 区间内的高度个数（含两端）
func (r Range) Count() int64 { return r.To.Int64() - r.From.Int64() + 1 }

func (r Range) String() string {
	return fmt.Sprintf("[%d, %d] 共 %d 个高度", r.From.Int64(), r.To.Int64(), r.Count())
}

// ResolveRange 解析并校验高度区间。
//
// 校验项（任一不满足即报错，绝不「猜一个默认区间」）：
//   - 起始高度必须 > 0（高度 0 无派生数据，且 0 常被当成「没传参数」的默认值）；
//   - 截止高度必须 > 0；
//   - 截止高度不得小于起始高度。
func ResolveRange(start, end int64) (Range, error) {
	if start <= 0 {
		return Range{}, fmt.Errorf("起始高度(--start)必须 > 0, 收到: %d", start)
	}
	if end <= 0 {
		return Range{}, fmt.Errorf("截止高度(--end)必须 > 0, 收到: %d", end)
	}
	if end < start {
		return Range{}, fmt.Errorf("截止高度(--end=%d)不能小于起始高度(--start=%d)", end, start)
	}
	return Range{From: chain.Epoch(start), To: chain.Epoch(end)}, nil
}

// RunOptions 离线回放的运行参数（由命令行装配，单测直接构造）
type RunOptions struct {
	From int64 // 起始高度（含）
	To   int64 // 截止高度（含）
	// NoWrite 只统计不落库：跑完整条 task 管线，但派生表写入（含删除）全部拦下只计数
	NoWrite         bool
	EpochsChunk     int64         // 并发同步高度数（<=0 取默认 3）
	EpochsThreshold int64         // 单批区间上限（<=0 取默认 20）
	ErrorWait       time.Duration // 单批失败后的重试等待（<=0 取默认 15s）
}

// Telemetry 离线回放的统计口径（聚合器调用次数 + 被拦下的派生表写入）
type Telemetry struct {
	From    int64
	To      int64
	NoWrite bool
	Agg     *CountingAgg
	// Repo 只有 --no-write 模式非 nil；真写模式下写入直接下发数据库，不计数
	Repo *NoWriteEvmTransferRepo
}

// BuildSyncer 组装离线回放同步器。
//
// 关键约定（离线回放的全部安全性都落在这一处）：
//   - WithDry(true)：Dry 模式下同步器只执行任务（= 派生表写入），**不写**
//     chain.sync_syncers（进度指针）/ chain.sync_task_epochs / chain.sync_syncer_epochs，
//     也不做链一致性检查与回滚；跳过台账 chain.sync_skipped_epochs 同样不会被写
//     （Dry 下 recordEpochFailure 直接返回，计数器永远不会达阈值，区间跳判定不成立）；
//   - WithInitEpoch / WithStopEpoch：只跑 [From, To] 这一段；
//   - SetTracesBuilder：本任务依赖 Datamap 里的 traces（与生产 evm-contract 同步器一致）；
//   - NoWrite：派生表仓储被 NoWriteEvmTransferRepo 包住，写入只计数不落库。
//
// inner 是派生表仓储的写入目标；传 nil 表示使用 dal.NewEVMTransferDal(db)（生产路径，
// 也是命令使用的路径）；单测传入假仓储以便断言「零写入」。
func BuildSyncer(opt RunOptions, db *gorm.DB, agg londobell.Agg, adapter londobell.Adapter,
	inner repository.EvmTransferRepo) (*syncer.Syncer, *Telemetry, error) {

	rng, err := ResolveRange(opt.From, opt.To)
	if err != nil {
		return nil, nil, err
	}
	if db == nil {
		return nil, nil, fmt.Errorf("离线回放需要数据库连接（--no-write 下也只读派生表，但仍需 DB 句柄）")
	}
	if agg == nil {
		return nil, nil, fmt.Errorf("离线回放需要聚合器客户端（traces/tipset 来源）")
	}
	if adapter == nil {
		return nil, nil, fmt.Errorf("离线回放需要适配器客户端（actor 状态来源）")
	}

	chunk, threshold, wait := opt.EpochsChunk, opt.EpochsThreshold, opt.ErrorWait
	if chunk <= 0 {
		chunk = defaultEpochsChunk
	}
	if threshold <= 0 {
		threshold = defaultEpochsThreshold
	}
	if wait <= 0 {
		wait = defaultErrorWait
	}

	cagg := NewCountingAgg(agg)
	tel := &Telemetry{From: rng.From.Int64(), To: rng.To.Int64(), NoWrite: opt.NoWrite, Agg: cagg}

	if inner == nil {
		inner = dal.NewEVMTransferDal(db)
	}
	var repo repository.EvmTransferRepo = inner
	if opt.NoWrite {
		nw := NewNoWriteEvmTransferRepo(inner)
		tel.Repo = nw
		repo = nw
	}

	from, to := rng.From.Int64(), rng.To.Int64()
	s := syncer.NewSyncer(
		// 与生产 evm-contract 同步器同名：任务名与表语义都按生产口径，Dry 保证不碰指针/台账
		syncer.WithName(syncer.EvmContractSyncer),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(cagg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(&from),
		syncer.WithStopEpoch(&to),
		syncer.WithEpochsChunk(chunk),
		syncer.WithEpochsThreshold(threshold),
		syncer.WithErrorWaitDuration(wait),
		syncer.WithDry(true),
		syncer.WithTaskGroup(
			[]syncer.Task{
				evm_transfer_task.NewEVMTransferTask(repo),
			},
		),
		syncer.WithContextBuilder(injector.SetTracesBuilder),
	)
	return s, tel, nil
}

// reportInput 报告的纯输入：把报告生成从运行态里剥出来，便于单测精确断言文案与数字。
type reportInput struct {
	From    int64
	To      int64
	NoWrite bool
	Elapsed time.Duration
	Agg     AggStats
	Writes  NoWriteStats
}

// Report 生成离线回放统计报告
func (t *Telemetry) Report(rng Range, elapsed time.Duration) string {
	in := reportInput{
		From:    rng.From.Int64(),
		To:      rng.To.Int64(),
		NoWrite: t.NoWrite,
		Elapsed: elapsed,
	}
	if t.Agg != nil {
		in.Agg = t.Agg.Stats()
	}
	if t.Repo != nil {
		in.Writes = t.Repo.Stats()
	}
	return formatReport(in)
}

func formatReport(in reportInput) string {
	var b strings.Builder
	b.WriteString("=== 离线回放统计报告（evm-transfer）===\n")
	fmt.Fprintf(&b, "同步器/任务     : %s / %s\n", syncer.EvmContractSyncer, replayTaskName)
	fmt.Fprintf(&b, "高度区间       : [%d, %d] 左闭右闭，共 %d 个高度\n", in.From, in.To, in.To-in.From+1)
	fmt.Fprintf(&b, "耗时           : %s\n", in.Elapsed.Round(time.Millisecond))
	fmt.Fprintf(&b, "聚合器调用     : Traces=%d Tipset=%d ParentTipset=%d LatestTipset=%d 合计=%d\n",
		in.Agg.Traces, in.Agg.Tipsets, in.Agg.ParentTipsets, in.Agg.LatestTipsets, in.Agg.Total())
	if in.NoWrite {
		b.WriteString("模式           : --no-write 只统计不落库（派生表写入被拦截；不写同步指针/台账/任务高度）\n")
		fmt.Fprintf(&b, "被拦下的写入   : fevm.evm_transfers %d 次/%d 行；"+
			"fevm.evm_transfer_stats %d 次/%d 行；删除 %d 次；合计 %d 行\n",
			in.Writes.TransferCalls, in.Writes.TransferRows,
			in.Writes.StatCalls, in.Writes.StatRows, in.Writes.DeleteCalls, in.Writes.TotalRows())
		fmt.Fprintf(&b, "预计写入行数   : 真写模式下即为 %d 行（本次未落库）\n", in.Writes.TotalRows())
	} else {
		b.WriteString("模式           : 真写（直接写 fevm.evm_transfers / fevm.evm_transfer_stats；" +
			"仍不写同步指针/台账/任务高度）\n")
		b.WriteString("派生表写入     : 已直接下发数据库，未计数\n")
	}
	return b.String()
}
