package offlinereplay

import (
	"fmt"
	"runtime/debug"
	"sync"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
)

// defaultAbortAfterFailures 清单模式下「同一高度连续失败多少次即放弃该高度」的默认值。
// 3 次 × errorWaitDuration（配置里的重试等待，内置默认 15s）≈ 45s/坏高度：
// 既不至于让一个坏高度拖死整批，也不会因为偶发抖动就放弃一个高度。
const defaultAbortAfterFailures = 3

// AbandonedEpoch 被放弃的高度（连续失败达上限，本轮不再重试）
type AbandonedEpoch struct {
	Epoch    int64  // 高度
	Attempts int    // 失败次数（= 该高度被执行过几次：被放弃意味着它一次都没成功）
	Err      string // 最后一次失败的错误文本
}

// epochGate 清单模式的「坏高度保险丝」。
//
// 为什么需要它：Dry 模式下同步器自带的坏高度处置**全是惰性的** ——
// modules/syncer/data_error.go 的 recordEpochFailure 首行是 `if s.dry || err == nil { return }`，
// 于是「连续 N 次失败即跳过」与「一次跳过整段」两条防线在 dry 下都不生效：
// 某个高度只要持续报错，同步器就按 errorWaitDuration 无限重试**同一个高度**，整批清单永远跑不完。
// 区间模式可以靠人盯（起点避开已知坏高度），无人值守的清单补数不行。
//
// 机制：把「同一高度的失败次数」数在任务/计算器的包装层里（gatedTask / gatedCalculator），
// 达上限后**吞掉错误**（返回 nil），让同步器认为该高度已处理、继续下一个高度；
// 被放弃的高度记进本对象，由报告逐条输出。
//
// 语义边界（别把「吞错误」误读成「数据补上了」）：
//   - 吞错误不写任何东西（Dry 模式本就不写 chain.sync_task_epochs / chain.sync_syncers / 跳过台账），
//     该高度在库里仍然是缺的 ⇒ 报告里被列成「放弃/未完成」，不会伪装成成功；
//   - 计数按高度累计，单进程内有效；清单模式下每个高度只跑一个同步器，不存在跨高度串味。
type epochGate struct {
	limit int

	mu       sync.Mutex
	attempts map[int64]int // 高度 → 失败次数
	aborted  []AbandonedEpoch
	inAbort  map[int64]struct{}
}

// newEpochGate 构造保险丝（limit <= 0 取默认 3）
func newEpochGate(limit int) *epochGate {
	if limit <= 0 {
		limit = defaultAbortAfterFailures
	}
	return &epochGate{
		limit:    limit,
		attempts: make(map[int64]int),
		inAbort:  make(map[int64]struct{}),
	}
}

// Limit 放弃阈值（同一高度连续失败多少次即放弃）
func (g *epochGate) Limit() int { return g.limit }

// observe 记录一次任务/计算器的执行结果：
//   - err == nil：原样返回 nil；
//   - err != nil 且该高度累计失败次数 < limit：原样返回 err（同步器按原有语义重试）；
//   - err != nil 且累计失败次数 >= limit：登记该高度并返回 nil（放弃它，让整批继续）。
func (g *epochGate) observe(epoch int64, err error) error {
	if err == nil {
		return nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()

	g.attempts[epoch]++
	n := g.attempts[epoch]
	if n < g.limit {
		return err
	}
	if _, ok := g.inAbort[epoch]; !ok {
		g.inAbort[epoch] = struct{}{}
		g.aborted = append(g.aborted, AbandonedEpoch{Epoch: epoch, Attempts: n, Err: err.Error()})
	}
	return nil
}

// Failures 某高度已记录的失败次数
func (g *epochGate) Failures(epoch int64) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.attempts[epoch]
}

// abandoned 该高度是否已被放弃（连续失败达上限，本轮不再重试）
func (g *epochGate) abandoned(epoch int64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	_, ok := g.inAbort[epoch]
	return ok
}

// Abandoned 被放弃的高度（按放弃顺序；返回副本）
func (g *epochGate) Abandoned() []AbandonedEpoch {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]AbandonedEpoch(nil), g.aborted...)
}

// wrap 给目标的任务与计算器套上保险丝（不改动调用方传入的 Target）
func (g *epochGate) wrap(t Target) Target {
	out := Target{Name: t.Name, SkipTraces: t.SkipTraces}
	for _, group := range t.Groups {
		wrapped := make(syncer.TaskGroup, 0, len(group))
		for _, task := range group {
			wrapped = append(wrapped, gatedTask{Task: task, gate: g})
		}
		out.Groups = append(out.Groups, wrapped)
	}
	for _, calc := range t.Calculators {
		out.Calculators = append(out.Calculators, gatedCalculator{Calculator: calc, gate: g})
	}
	return out
}

// gatedTask 任务包装：把 Exec 的失败按高度记进保险丝（Name/RollBack/HistoryClear 由嵌入接口透传）
type gatedTask struct {
	syncer.Task
	gate *epochGate
}

var _ syncer.Task = gatedTask{}

func (t gatedTask) Exec(ctx *syncer.Context) (err error) {
	epoch := ctx.Epoch().Int64()
	defer func() {
		// 任务内部 panic 若不在这里收敛，会被同步器的 recover 变成普通错误 ⇒ 又回到
		// 「无限重试同一高度」那条路上（保险丝看不到它）。这里自己 recover 并交给保险丝，
		// 同时把栈打出来，不丢现场。
		if e := recover(); e != nil {
			debug.PrintStack()
			err = t.gate.observe(epoch, fmt.Errorf("高度 %d 任务 %s panic: %v", epoch, t.Name(), e))
			return
		}
		if err != nil {
			err = t.gate.observe(epoch, err)
		}
	}()
	return t.Task.Exec(ctx)
}

// gatedCalculator 计算器包装：同上
type gatedCalculator struct {
	syncer.Calculator
	gate *epochGate
}

var _ syncer.Calculator = gatedCalculator{}

func (c gatedCalculator) Calc(ctx *syncer.Context) (err error) {
	epoch := ctx.Epoch().Int64()
	defer func() {
		if e := recover(); e != nil {
			debug.PrintStack()
			err = c.gate.observe(epoch, fmt.Errorf("高度 %d 计算器 %s panic: %v", epoch, c.Name(), e))
			return
		}
		if err != nil {
			err = c.gate.observe(epoch, err)
		}
	}()
	return c.Calculator.Calc(ctx)
}
