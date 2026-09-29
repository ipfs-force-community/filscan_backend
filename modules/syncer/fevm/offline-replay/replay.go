// Package offlinereplay 是「离线回放指定高度区间 / 指定高度清单的派生数据」工具链的公共框架。
//
// 子命令（evm-transfer / erc20 / fns / actor-actions）共用本包，各自只提供：
//   - 同步器名与任务/计算器（Target）；
//   - 「只统计不落库」的仓储包装（用本包的 Counters 记账）。
//
// 两种计划形态（见 epochlist.go 的 Plan）：
//   - 连续区间：--start/--end（左闭右闭），一个同步器跑完整段；
//   - 高度清单：--epochs-file（一行一个高度，可含空行/注释），升序逐个高度各跑一个同步器。
//     离散缺口（如某个派生表漏掉的几千个高度）只能用清单模式补 —— 按区间重跑会把整段一起重算。
//
// 公共安全性全部落在 Assemble 里（见其注释）：固定 Dry 模式 ⇒ 只跑任务与计算器
// （即派生表写入），不写 chain.sync_syncers 进度指针、不写 chain.sync_task_epochs /
// chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 台账，也不做链一致性检查与回滚。
package offlinereplay

import (
	"fmt"
	"strings"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
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
	From int64 // 起始高度（含）；区间模式下由 Execute 从 Plan 覆写
	To   int64 // 截止高度（含）；区间模式下由 Execute 从 Plan 覆写
	// NoWrite 只统计不落库：跑完整条 task/计算器管线，但派生表写入（含删除）全部拦下只计数
	NoWrite         bool
	EpochsChunk     int64         // 并发同步高度数（<=0 取默认 3）
	EpochsThreshold int64         // 单批区间上限（<=0 取默认 20）
	ErrorWait       time.Duration // 单批失败后的重试等待（<=0 取默认 15s）
	// EpochFailLimit 高度清单模式下「同一高度连续失败多少次即放弃该高度」的阈值（<=0 取默认 3）。
	// 区间模式不使用它（区间模式沿用「失败即整批重试」的原语义，见 epochgate.go 的说明）。
	EpochFailLimit int
}

// Target 回放目标：同步器名 + 任务分组 + 计算器 + 是否注入 traces 上下文。
// 各命令包按自身依赖构造（NoWrite 时把「只统计不落库」的仓储包装注进去）。
type Target struct {
	Name        string              // 与生产同步器同名的同步器名（如 evm-contract/erc20/fns/actor）
	Groups      []syncer.TaskGroup  // 任务分组（组间并发、组内串行，与生产一致）
	Calculators []syncer.Calculator // 计算器（按高度顺序串行执行）
	// SkipTraces 不注册 SetTracesBuilder（即不注入 Datamap 里的 traces）。
	// 默认 false = 注入（与生产 evm-contract / erc20 / fns 一致：它们的任务都读 traces）。
	// 置 true 仅当任务与计算器都不读 traces —— 生产 actor 同步器就是这种：
	// 它的 task/calculator 只用 adapter.ChangeActors + chain.actor_balances，没有 WithContextBuilder。
	// 置 true 同时省掉每个高度一次 Traces 调用（也避免 traces 取不到时整个高度失败）。
	SkipTraces bool
}

// TaskNames 本目标涉及的任务/计算器名（报告与日志用）
func (t Target) TaskNames() []string {
	var names []string
	for _, g := range t.Groups {
		for _, task := range g {
			names = append(names, task.Name())
		}
	}
	for _, c := range t.Calculators {
		names = append(names, c.Name())
	}
	return names
}

// Telemetry 离线回放的统计口径：聚合器调用次数 + 被拦下的派生表写入。
type Telemetry struct {
	From    int64
	To      int64
	NoWrite bool
	Syncer  string
	Tasks   []string
	Agg     *CountingAgg
	// List 高度清单模式下的清单（区间模式为 nil）。报告据此切换为逐高度结果输出。
	List *EpochList
	// Writes 写统计来源：由命令包返回各派生表的快照；真写模式返回 nil（写入直接下发，不计数）
	Writes func() []WriteStat
}

// Assemble 组装「连续区间」离线回放同步器。
//
// 关键约定（离线回放的全部安全性都落在这一处）：
//   - WithDry(true)：Dry 模式下同步器只执行任务与计算器（= 派生表写入），**不写**
//     chain.sync_syncers（进度指针）/ chain.sync_task_epochs / chain.sync_syncer_epochs，
//     也不做链一致性检查与回滚；跳过台账 chain.sync_skipped_epochs 同样不会被写
//     （Dry 下 recordEpochFailure 直接返回，失败计数永远不达阈值，区间跳判定不成立）；
//   - WithInitEpoch / WithStopEpoch：只跑 [From, To] 这一段；
//   - SetTracesBuilder（除非 Target.SkipTraces）：派生任务都依赖 Datamap 里的 traces（与生产一致）；
//   - 聚合器被 CountingAgg 包住：无论是否 --no-write，都会统计各方法的调用次数。
func Assemble(opt RunOptions, target Target, db *gorm.DB, agg londobell.Agg,
	adapter londobell.Adapter) (*syncer.Syncer, *Telemetry, error) {

	if agg == nil {
		return nil, nil, fmt.Errorf("离线回放需要聚合器客户端（traces/tipset 来源）")
	}
	cagg := NewCountingAgg(agg)
	s, rng, err := assembleRange(opt, target, db, cagg, adapter)
	if err != nil {
		return nil, nil, err
	}
	return s, &Telemetry{
		From:    rng.From.Int64(),
		To:      rng.To.Int64(),
		NoWrite: opt.NoWrite,
		Syncer:  target.Name,
		Tasks:   target.TaskNames(),
		Agg:     cagg,
	}, nil
}

// assembleRange 区间模式装配：校验区间与依赖，返回同步器与已校验的区间
func assembleRange(opt RunOptions, target Target, db *gorm.DB, cagg *CountingAgg,
	adapter londobell.Adapter) (*syncer.Syncer, Range, error) {

	rng, err := ResolveRange(opt.From, opt.To)
	if err != nil {
		return nil, Range{}, err
	}
	from, to := rng.From.Int64(), rng.To.Int64()
	opts, err := baseOptions(opt, target, db, cagg, adapter)
	if err != nil {
		return nil, Range{}, err
	}
	opts = append(opts, syncer.WithInitEpoch(&from), syncer.WithStopEpoch(&to))
	return syncer.NewSyncer(opts...), rng, nil
}

// assembleEpoch 清单模式装配：只跑一个高度（init == stop == epoch）。
// 该高度处理完（或经保险丝放弃）后 syncer 的 epoch 即越过 stopEpoch，Run() 自行返回 ——
// 一个高度一个同步器，于是「某高度失败后重试」只重跑这一个高度，不会像区间模式那样
// 把同批已成功的高度再写一遍（这也是带唯一键的派生表在区间重跑时会撞键的根因）。
func assembleEpoch(epoch int64, opt RunOptions, target Target, db *gorm.DB, cagg *CountingAgg,
	adapter londobell.Adapter) (*syncer.Syncer, error) {

	opts, err := baseOptions(opt, target, db, cagg, adapter)
	if err != nil {
		return nil, err
	}
	e := epoch
	opts = append(opts, syncer.WithInitEpoch(&e), syncer.WithStopEpoch(&e))
	return syncer.NewSyncer(opts...), nil
}

// baseOptions 两种计划形态共用的同步器配置（含依赖校验与默认值回退）
func baseOptions(opt RunOptions, target Target, db *gorm.DB, cagg *CountingAgg,
	adapter londobell.Adapter) ([]syncer.Option, error) {

	if db == nil {
		return nil, fmt.Errorf("离线回放需要数据库连接（--no-write 下也只读派生表，但仍需 DB 句柄）")
	}
	if cagg == nil {
		return nil, fmt.Errorf("离线回放需要聚合器客户端（traces/tipset 来源）")
	}
	if adapter == nil {
		return nil, fmt.Errorf("离线回放需要适配器客户端（actor/epoch 来源）")
	}
	if target.Name == "" {
		return nil, fmt.Errorf("离线回放需要同步器名（与生产同名，便于对照任务表语义）")
	}
	if len(target.Groups) == 0 && len(target.Calculators) == 0 {
		return nil, fmt.Errorf("离线回放同步器 %s 的任务与计算器都为空", target.Name)
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

	opts := []syncer.Option{
		syncer.WithName(target.Name),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(cagg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithEpochsChunk(chunk),
		syncer.WithEpochsThreshold(threshold),
		syncer.WithErrorWaitDuration(wait),
		syncer.WithDry(true),
		syncer.WithTaskGroup(target.Groups...),
		syncer.WithCalculators(target.Calculators...),
	}
	if !target.SkipTraces {
		opts = append(opts, syncer.WithContextBuilder(injector.SetTracesBuilder))
	}
	return opts, nil
}

// reportInput 报告的纯输入：把报告生成从运行态里剥出来，便于单测精确断言文案与数字。
type reportInput struct {
	From    int64
	To      int64
	NoWrite bool
	Syncer  string
	Tasks   []string
	Elapsed time.Duration
	Agg     AggStats
	Writes  []WriteStat
	// List + Result 非空 ⇒ 清单模式的报告（输出逐高度结果）
	List   *EpochList
	Result *listResult
}

// Report 生成离线回放统计报告（区间模式）
func (t *Telemetry) Report(rng Range, elapsed time.Duration) string {
	in := t.reportInputBase(elapsed)
	in.From, in.To = rng.From.Int64(), rng.To.Int64()
	return formatReport(in)
}

// ReportList 生成离线回放统计报告（清单模式，含逐高度结果）
func (t *Telemetry) ReportList(elapsed time.Duration, out listResult) string {
	in := t.reportInputBase(elapsed)
	in.List, in.Result = t.List, &out
	return formatReport(in)
}

func (t *Telemetry) reportInputBase(elapsed time.Duration) reportInput {
	in := reportInput{
		From:    t.From,
		To:      t.To,
		NoWrite: t.NoWrite,
		Syncer:  t.Syncer,
		Tasks:   t.Tasks,
		Elapsed: elapsed,
	}
	if t.Agg != nil {
		in.Agg = t.Agg.Stats()
	}
	if t.Writes != nil {
		in.Writes = t.Writes()
	}
	return in
}

func formatReport(in reportInput) string {
	var b strings.Builder
	b.WriteString("=== 离线回放统计报告 ===\n")
	fmt.Fprintf(&b, "同步器/任务     : %s / %s\n", in.Syncer, strings.Join(in.Tasks, "+"))
	if in.List != nil {
		fmt.Fprintf(&b, "高度清单       : %s\n", in.List)
	} else {
		fmt.Fprintf(&b, "高度区间       : [%d, %d] 左闭右闭，共 %d 个高度\n", in.From, in.To, in.To-in.From+1)
	}
	fmt.Fprintf(&b, "耗时           : %s\n", in.Elapsed.Round(time.Millisecond))
	fmt.Fprintf(&b, "聚合器调用     : Traces=%d Tipset=%d ParentTipset=%d LatestTipset=%d 合计=%d\n",
		in.Agg.Traces, in.Agg.Tipsets, in.Agg.ParentTipsets, in.Agg.LatestTipsets, in.Agg.Total())
	if in.NoWrite {
		b.WriteString("模式           : --no-write 只统计不落库（派生表写入被拦截；不写同步指针/台账/任务高度）\n")
		calls, rows, deletes := SumWrites(in.Writes)
		for _, w := range in.Writes {
			fmt.Fprintf(&b, "被拦下的写入   : %s 写 %d 次/%d 行；删除 %d 次\n",
				w.Table, w.Calls, w.Rows, w.Deletes)
		}
		fmt.Fprintf(&b, "被拦下的合计   : 写 %d 次/%d 行；删除 %d 次\n", calls, rows, deletes)
		fmt.Fprintf(&b, "预计写入行数   : 真写模式下即为 %d 行（本次未落库）\n", rows)
	} else {
		b.WriteString("模式           : 真写（直接写派生表；仍不写同步指针/台账/任务高度）\n")
		b.WriteString("派生表写入     : 已直接下发数据库，未计数\n")
	}
	if in.Result != nil {
		formatListOutcome(&b, in)
	}
	return b.String()
}

// maxReportedEpochs 报告里逐个列出的高度上限（其余只报个数，避免报告被几千行淹没）
const maxReportedEpochs = 10

// formatListOutcome 清单模式的逐高度结果段落
func formatListOutcome(b *strings.Builder, in reportInput) {
	r, list := in.Result, in.List
	total := 0
	if list != nil {
		total = list.Count()
	}
	fmt.Fprintf(b, "逐高度结果     : 已跑 %d/%d 个高度；放弃 %d 个；未跑 %d 个\n",
		r.Run, total, len(r.Abandoned), len(r.NotRun))
	if len(r.Abandoned) > 0 {
		fmt.Fprintf(b, "放弃的高度     : %d 个（同一高度连续失败达上限即放弃，避免一个坏高度拖死整批；"+
			"Dry 模式不写跳过台账，这些高度仍需补）\n", len(r.Abandoned))
		for i, a := range r.Abandoned {
			if i >= maxReportedEpochs {
				fmt.Fprintf(b, "  ... 其余 %d 个见日志（按高度 grep 即可）\n", len(r.Abandoned)-maxReportedEpochs)
				break
			}
			fmt.Fprintf(b, "  - 高度 %d 失败 %d 次，最后错误: %s\n", a.Epoch, a.Attempts, brief(a.Err))
		}
		b.WriteString("续跑提示       : 把上面列出的高度写进新的清单文件，用同一条命令重跑即可（清单模式无副作用，可重跑）\n")
	}
	if len(r.NotRun) > 0 {
		fmt.Fprintf(b, "未跑的高度     : %d 个（收到退出信号，从 %d 起）: %s\n",
			len(r.NotRun), r.NotRun[0], briefEpochs(r.NotRun))
	}
}

// brief 只保留错误文本的前 200 个字符（报告一行一条，过长的错误信息会挤掉版面）
func brief(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 200 {
		return s[:200] + "…"
	}
	return s
}

// briefEpochs 高度列表的紧凑写法（最多列 10 个）
func briefEpochs(epochs []int64) string {
	strs := make([]string, 0, maxReportedEpochs)
	for i, e := range epochs {
		if i >= maxReportedEpochs {
			strs = append(strs, fmt.Sprintf("…（共 %d 个）", len(epochs)))
			break
		}
		strs = append(strs, fmt.Sprintf("%d", e))
	}
	return strings.Join(strs, ", ")
}
