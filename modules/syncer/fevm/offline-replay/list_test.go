package offlinereplay

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// 本文件覆盖「按高度清单跑」（--epochs-file）的执行侧：
// 只跑清单里的高度（升序去重）、一条 SQL 都不下发、单个坏高度不阻断整批（含 panic）、
// 高于链头的高度在开跑前就被拦下、SkipTraces 目标不请求 traces。

// ---- 清单模式用的假任务/假计算器（不依赖 traces，便于同时覆盖 SkipTraces 目标）----

type listToy struct {
	name     string
	rows     int64 // 每次成功执行「写」几行
	counters *Counters

	mu       sync.Mutex
	perEpoch map[int64]int   // 高度 → 执行次数
	errs     map[int64]error // 该高度每次执行都报的错
	failOnce map[int64]bool  // 该高度只在第一次执行时报错（模拟偶发抖动，重试即恢复）
	panicAt  int64           // 该高度执行时 panic（0 = 不 panic）
}

func newListToy(name string, rows int64, counters *Counters) *listToy {
	return &listToy{name: name, rows: rows, counters: counters,
		perEpoch: map[int64]int{}, errs: map[int64]error{}, failOnce: map[int64]bool{}}
}

func (t *listToy) Name() string                                        { return t.name }
func (t *listToy) HistoryClear(_ context.Context, _ chain.Epoch) error { return nil }
func (t *listToy) RollBack(_ context.Context, _ chain.Epoch) error     { return nil }

func (t *listToy) run(epoch int64) error {
	t.mu.Lock()
	t.perEpoch[epoch]++
	err := t.errs[epoch]
	if err == nil && t.failOnce[epoch] {
		delete(t.failOnce, epoch)
		err = fmt.Errorf("高度 %d 第一次执行抖动（重试应恢复）", epoch)
	}
	doPanic := t.panicAt != 0 && t.panicAt == epoch
	t.mu.Unlock()

	if doPanic {
		panic(fmt.Sprintf("toy 故意 panic @ %d", epoch))
	}
	if err != nil {
		return err
	}
	t.counters.CountWrite(int(t.rows))
	return nil
}

// Exec 让 listToy 同时满足 syncer.Task
func (t *listToy) Exec(ctx *syncer.Context) error { return t.run(ctx.Epoch().Int64()) }

// Calc 让 listToy 同时满足 syncer.Calculator
func (t *listToy) Calc(ctx *syncer.Context) error { return t.run(ctx.Epoch().Int64()) }

// Runs 某高度被执行过的次数
func (t *listToy) Runs(epoch int64) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.perEpoch[epoch]
}

// RanEpochs 被执行过的高度（升序）
func (t *listToy) RanEpochs() []int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []int64
	for e, n := range t.perEpoch {
		if n > 0 {
			out = append(out, e)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// writeEpochList 造一份高度清单文件（原始文本原样写入，用于覆盖乱序/重复/注释/空行）
func writeEpochList(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	return path
}

// captureOutput 抓取 fn 期间写入 os.Stdout（报告，fmt.Println）与标准 logger（进度，log.Printf）的内容
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	oldStdout, oldLog := os.Stdout, log.Writer()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w
	log.SetOutput(w)
	defer func() {
		os.Stdout = oldStdout
		log.SetOutput(oldLog)
	}()

	fn()
	require.NoError(t, w.Close())
	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(out)
}

// 清单模式主用例：只跑清单里的高度（升序去重），一条 SQL 都不下发，跑完打印报告。
func TestListModeRunsExactlyListedEpochs(t *testing.T) {
	const head = 1000100

	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(head, nil)
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.derived")

	task := newListToy("list-toy-task", 1, counters)
	calc := newListToy("list-toy-calc", 2, counters)
	target := Target{
		Name:        "toy",
		Groups:      []syncer.TaskGroup{{task}},
		Calculators: []syncer.Calculator{calc},
	}

	listPath := writeEpochList(t, "# 缺口高度清单\n1000007\n1000003\n\n1000007   # 重复\n1000005\n")
	plan, err := ResolvePlan(0, 0, listPath)
	require.NoError(t, err)
	require.Equal(t, []int64{1000003, 1000005, 1000007}, plan.List.Epochs())

	report := captureOutput(t, func() {
		require.NoError(t, Execute(plan, RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
			target, db, agg, adapter, func() []WriteStat { return []WriteStat{counters.Snapshot()} }))
	})

	// 只跑清单里的高度：升序、去重、一个不多一个不少
	require.Equal(t, []int64{1000003, 1000005, 1000007}, task.RanEpochs())
	require.Equal(t, []int64{1000003, 1000005, 1000007}, calc.RanEpochs())
	for _, e := range []int64{1000003, 1000005, 1000007} {
		require.Equal(t, 1, task.Runs(e), "高度 %d 的任务应只执行一次", e)
		require.Equal(t, 1, calc.Runs(e), "高度 %d 的计算器应只执行一次", e)
	}

	// Dry 模式：同步指针 / 任务高度 / 跳过台账 / 派生表，一条 SQL 都不下发
	require.Empty(t, rec.Statements(), "清单模式同样不允许向数据库下发任何语句")
	begins, commits, rollbacks := rec.TxCounts()
	require.Equal(t, 6, begins, "3 个高度 × (1 任务 + 1 计算器) = 6 个事务")
	require.Equal(t, 6, commits)
	require.Zero(t, rollbacks)

	// 写统计是整批累计值：3 高度 × (任务 1 行 + 计算器 2 行)
	got := counters.Snapshot()
	require.Equal(t, int64(6), got.Calls)
	require.Equal(t, int64(9), got.Rows)

	require.Contains(t, report, "=== 离线回放统计报告 ===")
	require.Contains(t, report, "同步器/任务     : toy / list-toy-task+list-toy-calc")
	require.Contains(t, report, "高度清单       : "+listPath)
	require.Contains(t, report, "3 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 3/3 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "toy.derived 写 6 次/9 行；删除 0 次")
	require.NotContains(t, report, "放弃的高度     : ", "没有坏高度时报告不该出现放弃段落")
}

// 单个坏高度不阻断整批：该高度连续失败达上限后被放弃，其余高度照跑（含「抖一次就恢复」的
// 高度必须被认成完成），Execute 不返回错误。
func TestListModeBadHeightDoesNotBlockOthers(t *testing.T) {
	const head = 1000100
	const bad = 1000005
	const shaky = 1000009 // 第一次失败、重试即恢复

	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(head, nil)
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.derived")

	task := newListToy("list-toy-task", 1, counters)
	task.errs[bad] = fmt.Errorf("高度 %d 的 actor 余额尚未落库", bad)
	task.failOnce[shaky] = true
	calc := newListToy("list-toy-calc", 2, counters)
	target := Target{Name: "toy", Groups: []syncer.TaskGroup{{task}}, Calculators: []syncer.Calculator{calc}}

	plan, err := ResolvePlan(0, 0, writeEpochList(t, "1000003\n1000005\n1000007\n1000009\n"))
	require.NoError(t, err)

	report := captureOutput(t, func() {
		require.NoError(t, Execute(plan, RunOptions{
			NoWrite: true, ErrorWait: time.Millisecond, EpochFailLimit: 2,
		}, target, db, agg, adapter, func() []WriteStat { return []WriteStat{counters.Snapshot()} }))
	})

	// 坏高度：重试 2 次（达上限）后放弃；抖动高度：第 2 次成功；另外两个高度各跑一次
	require.Equal(t, 2, task.Runs(bad), "坏高度按上限重试 2 次后应被放弃")
	require.Equal(t, 2, task.Runs(shaky), "抖动高度重试一次即恢复")
	require.Equal(t, 1, task.Runs(1000003))
	require.Equal(t, 1, task.Runs(1000007))
	require.Equal(t, 1, calc.Runs(1000003))
	require.Equal(t, 1, calc.Runs(1000007))

	// 任务在三个健康高度各写 1 行；计算器四个高度都跑了（任务分组与计算器互相独立，
	// 坏高度的任务失败不影响它自己的计算器跑）⇒ 3 + 4×2 = 11 行。
	// 注意这不是「坏高度也补上了」：该高度的 task 侧数据仍然缺，保险丝只保证不阻断整批。
	require.Equal(t, int64(11), counters.Snapshot().Rows)
	require.Empty(t, rec.Statements())
	_, _, rollbacks := rec.TxCounts()
	require.GreaterOrEqual(t, rollbacks, 1, "失败的那次执行应回滚事务")

	require.Contains(t, report, "逐高度结果     : 已跑 4/4 个高度；放弃 1 个；未跑 0 个")
	require.Contains(t, report, "放弃的高度     : 1 个")
	require.Contains(t, report, fmt.Sprintf("- 高度 %d 失败 2 次", bad))
	require.Contains(t, report, "actor 余额尚未落库")
	require.Contains(t, report, "续跑提示")
	require.NotContains(t, report, fmt.Sprintf("高度 %d 失败", shaky), "抖动恢复的高度不算放弃")

	// 进度日志要把三种结局分清
	require.Contains(t, report, fmt.Sprintf("高度 %d 放弃: 失败 2 次达上限", bad))
	require.Contains(t, report, fmt.Sprintf("高度 %d 完成（重试 1 次后成功）", shaky))
	require.Contains(t, report, "高度 1000003 完成，耗时")
}

// 任务/计算器 panic 同样被保险丝收敛：不阻断整批，也不会让同步器原地重试到天荒地老。
func TestListModePanicIsGatedAndDoesNotHangTheBatch(t *testing.T) {
	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(1000100, nil)
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.derived")

	task := newListToy("list-toy-task", 1, counters)
	task.panicAt = 1000003
	target := Target{Name: "toy", Groups: []syncer.TaskGroup{{task}}}

	plan, err := ResolvePlan(0, 0, writeEpochList(t, "1000003\n1000005\n"))
	require.NoError(t, err)

	report := captureOutput(t, func() {
		require.NoError(t, Execute(plan, RunOptions{
			NoWrite: true, ErrorWait: time.Millisecond, EpochFailLimit: 2,
		}, target, db, agg, adapter, func() []WriteStat { return []WriteStat{counters.Snapshot()} }))
	})

	require.Equal(t, 2, task.Runs(1000003), "panic 高度被收敛成错误并计入保险丝")
	require.Equal(t, 1, task.Runs(1000005), "后续高度不受影响")
	require.Contains(t, report, "放弃的高度     : 1 个")
	require.Contains(t, report, "panic")
}

// 高过聚合器链头的高度在开跑前拦下（否则同步器会一直空转等待，整批永远跑不完）
func TestListModeRejectsHeightsAboveHead(t *testing.T) {
	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(1000006, nil) // 链头 = 1000005
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.derived")

	task := newListToy("list-toy-task", 1, counters)
	target := Target{Name: "toy", Groups: []syncer.TaskGroup{{task}}}

	plan, err := ResolvePlan(0, 0, writeEpochList(t, "1000003\n1000005\n1000007\n"))
	require.NoError(t, err)

	err = Execute(plan, RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		target, db, agg, adapter, nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "高于聚合器链头 1000005")
	require.Contains(t, err.Error(), "首个: 1000007")

	require.Empty(t, task.RanEpochs(), "链头校验失败时不得执行任何一个高度")
	require.Empty(t, rec.Statements())
	require.Zero(t, counters.Snapshot().Rows)
}

// SkipTraces 的目标（如生产 actor 同步器：计算器不读 traces）不应请求 traces
func TestSkipTracesTargetDoesNotRequestTraces(t *testing.T) {
	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(1000100, toyTraces(3))
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.derived")

	task := newListToy("list-toy-task", 1, counters)
	list, err := ParseEpochList("1000003\n1000005\n", "x")
	require.NoError(t, err)

	t.Run("SkipTraces=true：不请求 traces", func(t *testing.T) {
		plan := Plan{List: list, list: true}
		target := Target{Name: "toy", Groups: []syncer.TaskGroup{{task}}, SkipTraces: true}
		require.NoError(t, Execute(plan, RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
			target, db, agg, adapter, nil))
		require.Empty(t, agg.EpochsRequested(), "SkipTraces 目标不该请求 traces")
		require.Equal(t, []int64{1000003, 1000005}, task.RanEpochs())
	})

	t.Run("SkipTraces=false：每高度一次 traces（与生产 evm 系一致）", func(t *testing.T) {
		agg2 := NewFakeAgg(1000100, toyTraces(3))
		plan := Plan{List: list, list: true}
		target := Target{Name: "toy", Groups: []syncer.TaskGroup{{task}}}
		require.NoError(t, Execute(plan, RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
			target, db, agg2, adapter, nil))
		require.Equal(t, []int64{1000003, 1000005}, agg2.SortedEpochsRequested())
	})
}

// ---- 保险丝本体 ----

func TestEpochGateAbortsAfterLimit(t *testing.T) {
	gate := newEpochGate(3)
	require.Equal(t, 3, gate.Limit())

	errA := fmt.Errorf("a")
	require.Equal(t, errA, gate.observe(7, errA), "未达上限原样返回错误（同步器按原语义重试）")
	require.Equal(t, errA, gate.observe(7, errA))
	require.Equal(t, 2, gate.Failures(7))

	require.NoError(t, gate.observe(7, errA), "达上限即吞掉错误，让整批继续")
	require.Equal(t, 3, gate.Failures(7))
	require.Equal(t, []AbandonedEpoch{{Epoch: 7, Attempts: 3, Err: "a"}}, gate.Abandoned())

	// 再失败不会重复登记
	require.NoError(t, gate.observe(7, errA))
	require.Len(t, gate.Abandoned(), 1)

	// 其他高度独立计数；成功不计数
	require.NoError(t, gate.observe(8, nil))
	require.Zero(t, gate.Failures(8))
	require.Zero(t, gate.Failures(9))
	require.Error(t, gate.observe(9, errA))

	// 默认阈值
	require.Equal(t, defaultAbortAfterFailures, newEpochGate(0).Limit())
}

// 报告（清单模式）：被放弃的高度与未跑的高度都要写清楚，并给出续跑提示
func TestFormatListReport(t *testing.T) {
	list, err := ParseEpochList("# c\n6280001\n6280003\n6280005\n", "/tmp/gap.list")
	require.NoError(t, err)

	tel := &Telemetry{
		Syncer: "actor",
		Tasks:  []string{"calc-change-actor-task"},
		List:   &list,
	}
	report := tel.ReportList(1500*time.Millisecond, listResult{
		Run:       3,
		Abandoned: []AbandonedEpoch{{Epoch: 6280003, Attempts: 3, Err: "高度 6280003 的 actor 余额(chain.actor_balances)尚未落库"}},
		NotRun:    []int64{6280005},
	})

	require.Contains(t, report, "同步器/任务     : actor / calc-change-actor-task")
	require.Contains(t, report, "高度清单       : /tmp/gap.list")
	require.Contains(t, report, "3 个高度（升序去重后）")
	require.Contains(t, report, "耗时           : 1.5s")
	require.Contains(t, report, "逐高度结果     : 已跑 3/3 个高度；放弃 1 个；未跑 1 个")
	require.Contains(t, report, "- 高度 6280003 失败 3 次")
	require.Contains(t, report, "未跑的高度     : 1 个（收到退出信号，从 6280005 起）: 6280005")
	require.NotContains(t, report, "高度区间")
}

// 报告（清单模式）：放弃很多高度时只逐个列出前 10 个，避免报告被几千行淹没
func TestFormatListReportTruncatesAbandoned(t *testing.T) {
	list, err := ParseEpochList("1\n2\n3\n", "/tmp/gap.list")
	require.NoError(t, err)

	var aborted []AbandonedEpoch
	for i := int64(1); i <= 12; i++ {
		aborted = append(aborted, AbandonedEpoch{Epoch: i, Attempts: 3, Err: "boom"})
	}
	tel := &Telemetry{Syncer: "actor", Tasks: []string{"calc-change-actor-task"}, List: &list}
	report := tel.ReportList(time.Second, listResult{Run: 3, Abandoned: aborted})

	require.Contains(t, report, "放弃的高度     : 12 个")
	require.Contains(t, report, "- 高度 10 失败 3 次")
	require.NotContains(t, report, "- 高度 11 失败")
	require.Contains(t, report, "... 其余 2 个见日志")
}
