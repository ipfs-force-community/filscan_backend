// Package large_transfer_task 把「大额转账」按高度增量写进 PostgreSQL 表 chain.large_transfers。
//
// 背景（为什么需要这一步）：大额转账列表/计数原先每次都去冷库（11 个 mongo 分片）跑管道
// transfer_message_for_large_amount.js，命中面极小但必须靠 FIL 索引扫冷库再回表取文档，
// 实测单个请求 60 秒打不通；同一次请求还要在 12 个库上重算一遍命中行数。历史部分已一次性抽取进
// chain.large_transfers（DDL 见 migration/35.large_transfers.sql），本任务负责**增量**：
// 同步器处理到新高度时，把该高度新出现的大额转账顺带写进这张表，让读取侧始终只需要扫一张小表。
//
// 为什么挂在 chain 同步器的任务组里（injector.NewSyncerManager）：
// 该同步器已经注册了 injector.SetTracesBuilder，每个高度都会通过 ctx.Agg().Traces(epoch, epoch+1)
// 拿一份 traces 原始文档放进 Datamap（fevm 各子同步器、离线回放走的是同一条链）。本任务**只读
// Datamap 里现成的那份 traces**，因此不会新增任何聚合器请求（每高度的成本不翻倍）；换成新建一个
// 同步器、或者新起一次「按高度取 traces」，都会让每高度的聚合器调用翻倍，明确不做。
//
// 口径（与线上管道逐列对齐，证据都在聚合器仓的源码里）：
//   - 过滤：$match{"MsgRct.ExitCode": 0, "FIL": {$gte: 10000}}
//     （londobell-aggregators/pool-monitor/transfer_message_for_large_amount.js）。
//     traces 响应里**没有** FIL 字段（traces.js 的 $project 未投影 FIL），因此这里用
//     Msg.Value >= 1e22 attoFIL 判定，二者严格等价（证明见 isLargeAmountTransfer 的注释）。
//   - 列：Cid = SignedCid 优先否则 Cid；RootCid = RootSignedCid 优先否则 RootCid（traces 响应没有这两个
//     字段，按聚合器自己的算法从同高度 traces 里还原，见 rootCidIndex）；From/To = 地址原文
//     （mongo 存的是去掉网络前缀的形态）；Value = Msg.Value 的十进制原文（绝不经过浮点）；
//     Method = Msg.MethodName；Depth = Depth。
//
// 幂等与失败策略：
//   - 每个高度都是「同一事务内先 delete 后 insert」（repository.LargeTransferRepo），重复处理同一高度
//     （重试、回放）得到的结果与只处理一次完全相同；
//   - 写库失败**不返回错误**（只记日志 + 计数），绝不让本高度被判失败而无限重试、更不会把同步器卡死；
//     没写成功的高度按「重跑该高度」补齐（delete-then-insert 天然支持）。
package large_transfer_task

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	logging "github.com/gozelle/logger"
	"github.com/shopspring/decimal"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

const (
	// TaskName 任务唯一名（写进 chain.sync_task_epochs，参与一致性检查；只能小写字母/中横线/数字）
	TaskName = "large-transfers-task"

	// writeTimeout 单次写库的超时：把最坏情况限制在「一个高度被拖住 writeTimeout」，
	// 超时后按写失败处理（本高度继续，等重跑补齐）。
	writeTimeout = 15 * time.Second
)

// minAttoFil 金额门槛换算成 attoFIL：线上管道的门槛是 10000 FIL，
// 10000 FIL = 10000 * 1e18 = 1e22 attoFIL。
//
// decimal.New(1, 22) 是精确的 10^22（不是浮点 1e22），比较与写入全程只走整数/十进制字符串。
var minAttoFil = decimal.New(1, 22)

// logger 仅用于 RollBack（该方法拿不到 *syncer.Context，没有带高度/任务的 logger）
var logger = logging.NewLogger("large-transfer-task")

// NewLargeTransferTask 构造任务。
//
// enabled 就是配置项 syncer.sync_large_transfers（默认 false）：
//   - 注入侧（injector.NewSyncerManager）只在开启时注册本任务 ⇒ 关闭时**一个 SQL 都不发**、
//     连 chain.sync_task_epochs 都不会多一行，行为与打补丁前完全一致；
//   - 任务内部再判一次 enabled 属于双保险：将来若有别的装配路径（例如离线回放）无条件注册本任务，
//     开关关闭时也不会悄悄写库。
func NewLargeTransferTask(repo repository.LargeTransferRepo, enabled bool) *LargeTransferTask {
	return &LargeTransferTask{repo: repo, enabled: enabled}
}

var _ syncer.Task = (*LargeTransferTask)(nil)

type LargeTransferTask struct {
	repo    repository.LargeTransferRepo
	enabled bool

	// mu 保护下面这些可观测状态：同一个任务实例会被**多个高度并发**调用（syncer 按 epochsChunk 并发）
	mu                 sync.Mutex
	failedEpochs       int64 // 写库失败的高度次数（累计，供日志/监控/单测断言）
	missingTableWarned bool  // 「表不存在」是否已经告警过（只 Warn 一次，不刷屏）
	missingTableWarns  int64 // 「表不存在」实际打印的告警次数（应恒为 0 或 1）
	noTracesWarned     bool  // 「Datamap 里没有 traces」是否已经告警过（同上）
	noTracesWarns      int64 // 同上，实际打印次数
}

// FailedEpochs 写库失败的高度次数（累计）。日志之外留一个数字，便于监控/单测断言「失败不静默」。
func (t *LargeTransferTask) FailedEpochs() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.failedEpochs
}

// MissingTableWarns 「表不存在」的告警打印次数（正常应为 0 或 1 —— 只 Warn 一次不刷屏）
func (t *LargeTransferTask) MissingTableWarns() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.missingTableWarns
}

// NoTracesWarns 「Datamap 里没有 traces」的告警打印次数（同上）
func (t *LargeTransferTask) NoTracesWarns() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.noTracesWarns
}

func (t *LargeTransferTask) Name() string {
	return TaskName
}

// HistoryClear 本表按高度整体替换、不保留历史快照，统一清理历史数据时无需额外动作。
func (t *LargeTransferTask) HistoryClear(_ context.Context, _ chain.Epoch) error {
	return nil
}

// RollBack 链回滚：删除 >= gteEpoch 的行。
//
// 与 Exec 同一条失败策略：**不把错误上抛**。上抛会让 syncer.Rollback 失败，同步器就卡在
// 「回滚-重试」循环里（比留下几行陈旧数据严重得多）；而没删掉的行会在这些高度重新同步时被
// delete-then-insert 覆盖 —— 语义上自愈，所以这里只记日志 + 计数。
func (t *LargeTransferTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	if !t.enabled || t.repo == nil {
		return nil
	}
	if e := t.repo.DeleteLargeTransfersFromEpoch(ctx, gteEpoch); e != nil {
		t.recordFailure()
		logger.Errorf("大额转账增量回滚失败（chain.large_transfers, epoch >= %s，已忽略，不影响回滚流程）: %s；"+
			"这些高度重新同步时会按 delete-then-insert 覆盖", gteEpoch, e)
		return nil
	}
	return nil
}

// Exec 把该高度的大额转账增量写进 chain.large_transfers。
//
// 返回值恒为 nil（除非入参本身缺失）：本步骤是「顺带维护」，任何失败都不得影响本高度其余任务，
// 更不得让该高度被判失败而无限重试。
func (t *LargeTransferTask) Exec(ctx *syncer.Context) (err error) {
	if !t.enabled || t.repo == nil {
		return nil
	}
	if ctx == nil || ctx.Empty() {
		return nil
	}
	// Dry 模式（离线回放）的语义是「照样执行任务与计算器（= 派生表写入），只是不写同步指针/台账」，
	// 所以本任务在 Dry 下**照常写**，没有需要额外跳过的动作。

	// 最后一道防线：即使出现意料外的 panic（畸形 trace），也只记日志，不把整个任务组打挂
	// （execTaskOrCalculator 的 recover 会把 panic 变成错误 ⇒ 该高度按数据级错误反复重试甚至登记跳过）。
	defer func() {
		if e := recover(); e != nil {
			ctx.Errorf("高度 %s 大额转账增量维护 panic（已忽略，不影响本高度其余任务）: %v", ctx.Epoch(), e)
		}
	}()

	traces, ok := t.tracesFromDatamap(ctx)
	if !ok || len(traces) == 0 {
		// traces 为空有两种可能：① 空高度（SetTracesBuilder 在 ctx.Empty() 时写入 nil）；
		// ② 该高度确实没有任何消息。两种情况都**不写库**：这里刻意不做删除 —— 空/缺失的 traces
		// 无法证明「该高度本来就没有大额转账」，而误删会把一次性回填的历史行抹掉。
		// 链重组导致某高度变空的情形由 RollBack（删除 >= 回滚高度）覆盖，不会留下陈旧行。
		ctx.Debugf("高度 %s traces 为空，跳过 chain.large_transfers 增量（不下发 SQL）", ctx.Epoch())
		return nil
	}

	epoch := ctx.Epoch().Int64()
	rows := SelectLargeTransfers(epoch, traces)

	// 写库用**自己的** context/事务，不是 ctx.Context()：
	// syncer.execTaskOrCalculator 在 ctx.Context() 里挂着一个 PG 事务，一旦写失败该事务就进入
	// aborted 状态，随后它的 Commit 必然报错 ⇒ 该高度被判失败并无限重试。用独立事务 + 吃掉错误，
	// 才能满足「写 PG 失败绝不能把管线卡死」。
	wctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
	defer cancel()

	if err := t.repo.ReplaceLargeTransfers(wctx, epoch, rows); err != nil {
		t.recordFailure()
		switch {
		case isMissingTableErr(err):
			// 表不存在（migration/35.large_transfers.sql 还没执行）：只 Warn 一次，之后降级成 Debug，
			// 既不刷屏也不静默。
			if t.markMissingTableWarned() {
				ctx.Errorf("高度 %s 大额转账增量写库失败：目标表 chain.large_transfers 不存在（第 %d 次失败）；"+
					"请先执行表结构（另一条工作线：migration/35.large_transfers.sql）；在表就绪前，这些高度需要重跑补齐",
					epoch, t.FailedEpochs())
			} else {
				ctx.Debugf("高度 %s chain.large_transfers 仍不存在（累计 %d 个高度未写入），失败明细不再重复打印",
					epoch, t.FailedEpochs())
			}
		default:
			ctx.Errorf("高度 %s 大额转账增量写库失败（累计第 %d 个失败高度）: %s；"+
				"本高度继续往下走，该高度的增量需重跑补齐（delete-then-insert 语义天然可重跑）",
				epoch, t.FailedEpochs(), err)
		}
		return nil
	}

	if len(rows) > 0 {
		ctx.Infof("高度 %s 大额转账增量已写库: %d 行（同一事务内 delete-then-insert）", ctx.Epoch(), len(rows))
	} else {
		ctx.Debugf("高度 %s 大额转账增量写库: 0 行（traces %d 条中无命中）", ctx.Epoch(), len(traces))
	}
	return nil
}

// tracesFromDatamap 取本高度的 traces（SetTracesBuilder 放的原始文档）。
//
// 取不到时只 Warn 一次并返回 ok=false：本任务不返回错误，因为「Datamap 里没有 traces」= 当前同步器
// 没配 SetTracesBuilder，这是装配问题而不是数据问题，返回错误只会让该高度被判失败并无限重试。
func (t *LargeTransferTask) tracesFromDatamap(ctx *syncer.Context) (traces []*londobell.TraceMessage, ok bool) {
	val, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		if t.markNoTracesWarned() {
			ctx.Warnf("高度 %s Datamap 里没有 traces（本同步器是否忘了 WithContextBuilder(SetTracesBuilder)？），"+
				"本次跳过 chain.large_transfers 增量: %s", ctx.Epoch(), err)
		}
		return nil, false
	}

	// 逗号-ok 断言：ctx.Empty() 时 SetTracesBuilder 会写入 nil，直接类型断言会 panic。
	traces, ok = val.([]*londobell.TraceMessage)
	if !ok {
		if t.markNoTracesWarned() {
			ctx.Warnf("高度 %s Datamap 里的 traces 类型不是 []*londobell.TraceMessage，本次跳过 chain.large_transfers 增量", ctx.Epoch())
		}
		return nil, false
	}
	return traces, true
}

func (t *LargeTransferTask) recordFailure() {
	t.mu.Lock()
	t.failedEpochs++
	t.mu.Unlock()
}

// markMissingTableWarned 返回是否是首次（true = 本次由调用方打印）；首次会累加告警计数
func (t *LargeTransferTask) markMissingTableWarned() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.missingTableWarned {
		return false
	}
	t.missingTableWarned = true
	t.missingTableWarns++
	return true
}

// markNoTracesWarned 返回是否是首次（true = 本次由调用方打印）；首次会累加告警计数
func (t *LargeTransferTask) markNoTracesWarned() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.noTracesWarned {
		return false
	}
	t.noTracesWarned = true
	t.noTracesWarns++
	return true
}

// isMissingTableErr 判定「目标表不存在」（PostgreSQL SQLSTATE 42P01 undefined_table / relation ... does not exist）。
// 只认 SQLSTATE 与明确的关系名文案，避免把别的错误（连接断开、权限、超时）误判成表缺失。
func isMissingTableErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	if strings.Contains(msg, "42P01") {
		return true
	}
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "does not exist") && strings.Contains(lower, "large_transfers")
}

// SelectLargeTransfers 从某高度的 traces 里挑出要写进 chain.large_transfers 的行（纯函数，便于单测）。
//
// 与线上管道等价的判据见 isLargeAmountTransfer；列映射见 cidOf / rootCidOf / methodOf / transferValue。
func SelectLargeTransfers(epoch int64, traces []*londobell.TraceMessage) (rows []*po.LargeTransfer) {
	rootIndex := rootCidIndex(traces)
	for _, trace := range traces {
		if !isLargeAmountTransfer(trace) {
			continue
		}
		rows = append(rows, &po.LargeTransfer{
			Epoch:    epoch,
			Cid:      cidOf(trace),
			RootCid:  rootCidOf(trace, rootIndex),
			FromAddr: transferAddress(trace, true),
			ToAddr:   transferAddress(trace, false),
			Value:    transferValue(trace).String(),
			Method:   methodOf(trace),
			Depth:    trace.Depth,
		})
	}
	return rows
}

// isLargeAmountTransfer 判定该 trace 是否命中线上大额转账管道。
//
// 线上管道（londobell-aggregators/pool-monitor/transfer_message_for_large_amount.js）的两个 $match 条件：
//
//	"MsgRct.ExitCode": 0        ← mongo 语义：只命中「MsgRct 存在且 ExitCode==0」的文档
//	"FIL": {$gte: 10000}
//
// 第一条这里用 `MsgRct != nil && ExitCode == 0` 复刻（缺 MsgRct 的文档在 mongo 里不匹配，不是按 0 处理）。
//
// 第二条：traces 响应里没有 FIL（traces.js 的 $project 只投影 ID/Cid/SignedCid/Epoch/Seq/Depth/Ver/
// Msg/MsgRct/Error/SeqIndex/SubCallCount/GasCost/ReturnBson/Version/To/From/Nonce/Value/…），
// 所以用 FIL 的**定义**换过来算。FIL 是写入 mongo 时算好的整型字段：
//
//	me.FIL = CalculateFILValue(me.Msg.Value.String())
//	// CalculateFILValue: 取十进制文本去掉末尾 18 位（= 去掉 attoFIL 的小数部分）解析成 int64
//	//   londobell racailum/segment/model/exec_trace.go:80 与 :273
//
// 于是对非负整数 attoFIL 值 V（mongo 里就是 big.Int 的十进制文本）：
//
//	FIL >= 10000  ⟺  floor(V / 1e18) >= 10000  ⟺  V >= 10000 * 1e18 = 1e22
//
// 即 FIL >= 10000 与 V >= 1e22 attoFIL **严格等价**（边界：1e22 命中，9999999999999999999999 不命中）。
// 这里用 Value 直接比较，整数/十进制比较，不引入任何浮点。
func isLargeAmountTransfer(trace *londobell.TraceMessage) bool {
	if trace == nil || trace.MsgRct == nil {
		return false
	}
	if trace.MsgRct.ExitCode != 0 {
		return false
	}
	return transferValue(trace).GreaterThanOrEqual(minAttoFil)
}

// transferValue 取金额（attoFIL 十进制）。
//
// 首选 Msg.Value：线上管道的 value 列与 FIL 判据都取自它（$Msg.Value），而且 FIL 本身就是用它算出来的
// ⇒ 用它与线上逐字一致。Msg.Value 缺失（响应里没有该字段）时退回 traces 管道一并返回的 message.Value
// （同一条消息的同一金额，鉴于是兜底路径而不是主路径）。
func transferValue(trace *londobell.TraceMessage) (v decimal.Decimal) {
	if trace.Msg != nil && trace.Msg.Value != nil {
		return *trace.Msg.Value
	}
	return trace.Value
}

// methodOf 取方法名：线上列取自 $Msg.MethodName，缺失时退回 traces 里的 Detail.Method。
func methodOf(trace *londobell.TraceMessage) string {
	if trace.Msg != nil && trace.Msg.MethodName != "" {
		return trace.Msg.MethodName
	}
	if trace.Detail != nil {
		return trace.Detail.Method
	}
	return ""
}

// transferAddress 取转账的 from（isFrom=true）或 to 地址，写成 mongo 里存的「不带网络前缀」形态。
//
// 取值来源与线上管道一致：管道投影的是 $Msg.From / $Msg.To（transfer_message_for_large_amount.js 的
// $project），traces 响应里 Msg 子文档是整份投影的，所以正常情况下都从 Msg 取；
// Msg 缺失或该字段为空（极少数畸形文档）时退回 trace 自己的 From/To（同一条消息的同一地址）——
// 空地址永远不是正确答案，能取到就取。
func transferAddress(trace *londobell.TraceMessage, isFrom bool) string {
	var from, to chain.SmartAddress
	if trace.Msg != nil {
		from, to = trace.Msg.From, trace.Msg.To
	}
	if from == "" {
		from = trace.From
	}
	if to == "" {
		to = trace.To
	}
	if isFrom {
		return crudeAddress(from)
	}
	return crudeAddress(to)
}

// crudeAddress 把地址还原成「不带网络前缀」的原文（mongo 侧 addressBSONEncode = addr.String()[1:]）。
//
// 为什么要包一层而不是直接调 chain.SmartAddress.CrudeAddress()：
// CrudeAddress() 先过 Address()，而 Address() 对**已经没有前缀**的输入返回空串（它只剥不补），
// 于是一旦上游给的就是 "1abc" 这种原文，CrudeAddress() 会得到 ""——那会把 from_addr/to_addr 写成空串。
// 这里两种输入都接受：带前缀（"f1abc"/"t3def"）⇒ 去掉首字符；已经是原文 ⇒ 原样写出。
func crudeAddress(addr chain.SmartAddress) string {
	if crude := addr.CrudeAddress(); crude != "" {
		return crude
	}
	return string(addr)
}

// cidOf 取消息 cid：SignedCid 优先，否则 Cid（与线上管道的 $cond 一致；空串也回退，
// 否则会写出空 cid 的行）。与 modules/syncer/fevm/evm-transfer-task 的写法一致。
func cidOf(trace *londobell.TraceMessage) string {
	if trace.SignedCid != nil && *trace.SignedCid != "" {
		return *trace.SignedCid
	}
	return trace.Cid
}

// rootCidIndex 同高度 traces 的「根消息 cid」索引：Seq 首元素 → 该根消息的 cid。
//
// 为什么需要它：线上管道的 RootCid 取自 mongo 文档的 RootSignedCid/RootCid 字段，而 traces 管道
// 没有投影这两个字段。这两个字段在写入 mongo 时是这么算的（londobell racailum/segment/model/exec_trace.go）：
//
//	// 只有「块级消息」（IsBlock）才会被登记进 IDCidMap
//	if met.IsBlock { IDCidMap[met.ID] = [2]cid.Cid{met.Cid, met.SignedCid} }
//	// 子调用：根 ID = GetRootID(自己的 ID) = 自己的 ID 的前两段（= 同一区块消息树的第一层）
//	et.RootCid, et.RootSignedCid = m[rootID][0], m[rootID][1]      // IsBlock 的文档直接 return（不设值）
//
// 而 ExecTrace._id = "<Epoch>-<Seq 各元素补零后以 - 连接>"（exec_trace.go genID），
// GetRootID 取的就是「Epoch + Seq 首元素」⇒ 根消息 = 同一高度里 Seq 首元素相同、且长度为 1 的那条 trace。
// 于是这里按同一规则建索引：只有 IsBlock 的 trace 才是根候选。IsBlock 的定义见
// londobell racailum/segment/extract/tipset/tipset.go:IsBlock（len(Seq)==1 且 From 是 robust 地址）。
//
// 线上投影对这个字段的取值是「RootSignedCid 优先，否则 RootCid」，即根消息的 (SignedCid ?? Cid)，
// 所以索引里直接存 cidOf(根 trace)。
func rootCidIndex(traces []*londobell.TraceMessage) map[string]string {
	index := map[string]string{}
	for _, trace := range traces {
		if trace == nil || !trace.IsBlock || len(trace.Seq) == 0 {
			continue
		}
		index[seqKey(trace.Seq)] = cidOf(trace)
	}
	return index
}

// rootCidOf 还原该 trace 的 root_cid；没有对应根消息时返回 nil（写 NULL）。
//
// 与 mongo 的算法一一对应：
//   - 根消息自身（IsBlock / Seq 只有 1 段）：mongo 端 genRootids 直接 return，两个字段都没被写入，
//     线上投影得到 null ⇒ 这里也写 NULL（一次性回填的历史行同样是 NULL）；
//   - 子调用（Seq 多段）：取「同一高度、Seq 首元素相同、IsBlock」的根消息的 (SignedCid ?? Cid)；
//     找不到（根消息不在本批 traces 里）时写 NULL。
func rootCidOf(trace *londobell.TraceMessage, index map[string]string) *string {
	if trace == nil || trace.IsBlock || len(trace.Seq) <= 1 {
		return nil
	}
	root, ok := index[seqKey(trace.Seq[:1])]
	if !ok || root == "" {
		return nil
	}
	rootCid := root
	return &rootCid
}

// seqKey 把 Seq 的首元素转成索引键（对应 mongo 里 GetRootID 取 ID 前两段的行为）
func seqKey(seq []int64) string {
	if len(seq) == 0 {
		return ""
	}
	return strconv.FormatInt(seq[0], 10)
}
