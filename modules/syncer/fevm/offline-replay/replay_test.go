package offlinereplay

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// stateTreeError 与生产中「缺口高度在节点侧读不到历史状态」的错误同形
// （modules/syncer/data_error.go 的 unrecoverableStateTexts 命中项）。
func stateTreeError() error {
	return fmt.Errorf("failed to load state tree: failed to load hamt node bafybeidreadbeefdeadbeef: not found")
}

func TestResolveRange(t *testing.T) {
	cases := []struct {
		name    string
		start   int64
		end     int64
		wantErr string
		want    Range
	}{
		{name: "正常区间", start: 6357058, end: 6408647, want: Range{From: 6357058, To: 6408647}},
		{name: "单高度区间", start: 6, end: 6, want: Range{From: 6, To: 6}},
		{name: "最小合法高度", start: 1, end: 1, want: Range{From: 1, To: 1}},
		{name: "跨 120 整数倍高度", start: 600, end: 604, want: Range{From: 600, To: 604}},
		{name: "起始高度为 0", start: 0, end: 10, wantErr: "起始高度(--start)必须 > 0"},
		{name: "起始高度为负", start: -1, end: 10, wantErr: "起始高度(--start)必须 > 0"},
		{name: "截止高度为 0", start: 1, end: 0, wantErr: "截止高度(--end)必须 > 0"},
		{name: "截止高度为负", start: 1, end: -5, wantErr: "截止高度(--end)必须 > 0"},
		{name: "截止早于起始", start: 6408647, end: 6357058, wantErr: "不能小于起始高度"},
		{name: "起止都非法时先报起始", start: 0, end: 0, wantErr: "起始高度(--start)必须 > 0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ResolveRange(c.start, c.end)
			if c.wantErr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), c.wantErr)
				require.Equal(t, Range{}, got, "校验失败时不得返回半成品区间")
				return
			}
			require.NoError(t, err)
			require.Equal(t, c.want, got)
			require.Equal(t, c.end-c.start+1, got.Count())
			require.Equal(t, fmt.Sprintf("[%d, %d] 共 %d 个高度", c.start, c.end, c.end-c.start+1), got.String())
		})
	}
}

// ---- 框架管线用例用的假任务：只做「把 traces 行数记到计数仓储」这一件事 ----

type toySink struct {
	counters *Counters

	mu       sync.Mutex
	fails    int
	failOnce map[int64]bool
}

func (s *toySink) markFail(epoch int64) bool { // 返回 true = 本次该报错
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failOnce == nil {
		s.failOnce = map[int64]bool{}
	}
	if s.failOnce[epoch] {
		return false
	}
	s.failOnce[epoch] = true
	s.fails++
	return true
}

// Failures 假任务报错次数
func (s *toySink) Failures() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.fails
}

type toyTask struct {
	sink      *toySink
	failEpoch int64 // 该高度第一次执行时故意报错（0 = 不报错）
	failErr   error
}

func (t toyTask) Name() string                                        { return "toy-task" }
func (t toyTask) HistoryClear(_ context.Context, _ chain.Epoch) error { return nil }
func (t toyTask) RollBack(_ context.Context, _ chain.Epoch) error     { return nil }

func (t toyTask) Exec(ctx *syncer.Context) error {
	if ctx.Empty() {
		return nil
	}
	if t.failEpoch != 0 && ctx.Epoch().Int64() == t.failEpoch && t.sink.markFail(ctx.Epoch().Int64()) {
		return t.failErr
	}
	val, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return err
	}
	traces := val.([]*londobell.TraceMessage)
	t.sink.counters.CountWrite(len(traces)) // 模拟「每条 trace 一行派生数据」
	return nil
}

type toyCalc struct{}

func (c toyCalc) Name() string                                        { return "toy-calc" }
func (c toyCalc) HistoryClear(_ context.Context, _ chain.Epoch) error { return nil }
func (c toyCalc) RollBack(_ context.Context, _ chain.Epoch) error     { return nil }
func (c toyCalc) Calc(_ *syncer.Context) error                        { return nil }

func toyTraces(n int) func(chain.Epoch) []*londobell.TraceMessage {
	return func(epoch chain.Epoch) []*londobell.TraceMessage {
		out := make([]*londobell.TraceMessage, 0, n)
		for i := 0; i < n; i++ {
			out = append(out, &londobell.TraceMessage{
				Cid:   fmt.Sprintf("bafy-%d-%d", epoch.Int64(), i),
				Epoch: epoch.Int64(),
			})
		}
		return out
	}
}

func TestTargetTaskNames(t *testing.T) {
	target := Target{
		Name:        "toy",
		Groups:      []syncer.TaskGroup{{toyTask{}, toyTask{}}},
		Calculators: []syncer.Calculator{toyCalc{}},
	}
	require.Equal(t, []string{"toy-task", "toy-task", "toy-calc"}, target.TaskNames())
}

func TestAssembleRejectsInvalidInputs(t *testing.T) {
	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(100000, nil)
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.table")
	valid := Target{Name: "toy", Groups: []syncer.TaskGroup{{toyTask{sink: &toySink{counters: counters}}}}}

	t.Run("区间非法", func(t *testing.T) {
		_, _, err := Assemble(RunOptions{From: 10, To: 5}, valid, db, agg, adapter)
		require.Error(t, err)
		require.Contains(t, err.Error(), "不能小于起始高度")
	})
	t.Run("缺数据库", func(t *testing.T) {
		_, _, err := Assemble(RunOptions{From: 1, To: 2}, valid, nil, agg, adapter)
		require.Error(t, err)
		require.Contains(t, err.Error(), "数据库连接")
	})
	t.Run("缺聚合器", func(t *testing.T) {
		_, _, err := Assemble(RunOptions{From: 1, To: 2}, valid, db, nil, adapter)
		require.Error(t, err)
		require.Contains(t, err.Error(), "聚合器")
	})
	t.Run("缺适配器", func(t *testing.T) {
		_, _, err := Assemble(RunOptions{From: 1, To: 2}, valid, db, agg, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "适配器")
	})
	t.Run("缺同步器名", func(t *testing.T) {
		_, _, err := Assemble(RunOptions{From: 1, To: 2}, Target{Groups: valid.Groups}, db, agg, adapter)
		require.Error(t, err)
		require.Contains(t, err.Error(), "同步器名")
	})
	t.Run("无任务无计算器", func(t *testing.T) {
		_, _, err := Assemble(RunOptions{From: 1, To: 2}, Target{Name: "toy"}, db, agg, adapter)
		require.Error(t, err)
		require.Contains(t, err.Error(), "任务与计算器都为空")
	})
	t.Run("参数齐全时默认值可回退", func(t *testing.T) {
		s, tel, err := Assemble(RunOptions{From: 1, To: 2}, valid, db, agg, adapter)
		require.NoError(t, err) // EpochsChunk/Threshold/ErrorWait 为 0 时回退默认值
		require.NotNil(t, s)
		require.False(t, tel.NoWrite)
		require.Equal(t, "toy", tel.Syncer)
		require.Equal(t, []string{"toy-task"}, tel.Tasks)
		require.NoError(t, s.Init())
	})
}

// 框架主用例：Dry 模式跑满 [from, to]，要求
//   - 一条 SQL 都不下发（同步指针 / 任务高度 / 台账 / 派生表全部未写）；
//   - 计数仓储（派生表写入目标）被如实记账；
//   - 区间边界恰好 [from, to]（左闭右闭，不多跑一个高度）。
func TestDryPipelineRunsWholeRangeWithoutSQL(t *testing.T) {
	const from, to = 6, 10

	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(100000, toyTraces(2))
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.derived")
	target := Target{
		Name:        "toy",
		Groups:      []syncer.TaskGroup{{toyTask{sink: &toySink{counters: counters}}}},
		Calculators: []syncer.Calculator{toyCalc{}},
	}

	s, tel, err := Assemble(RunOptions{
		From: from, To: to, NoWrite: true,
		EpochsChunk: 2, EpochsThreshold: 5, ErrorWait: time.Millisecond,
	}, target, db, agg, adapter)
	require.NoError(t, err)
	tel.Writes = func() []WriteStat { return []WriteStat{counters.Snapshot()} }
	require.NoError(t, s.Init())

	RunSyncerAndWait(t, s.Run, 60*time.Second)

	require.Empty(t, rec.Statements(), "离线回放不允许向数据库下发任何语句")
	// Dry 模式下按「每个（任务|计算器）× 每个高度」各开一个事务：
	// 本用例每高度 1 个任务 + 1 个计算器 ⇒ 每高度 2 个事务。真写模式才会在同一事务里
	// 追加任务高度（chain.sync_task_epochs）与同步器进度（chain.sync_syncer_epochs）写入。
	begins, commits, rollbacks := rec.TxCounts()
	require.Equal(t, 2*5, begins)
	require.Equal(t, 2*5, commits)
	require.Zero(t, rollbacks)
	require.Equal(t, []int64{from, from + 1, from + 2, from + 3, to}, agg.SortedEpochsRequested())

	got := counters.Snapshot()
	require.Equal(t, int64(5), got.Calls)
	require.Equal(t, int64(10), got.Rows)
	require.Zero(t, got.Deletes)

	require.Equal(t, AggStats{Traces: 5, Tipsets: 10, ParentTipsets: 5, LatestTipsets: 1}, tel.Agg.Stats())

	report := tel.Report(Range{From: from, To: to}, 2*time.Second)
	require.Contains(t, report, "同步器/任务     : toy / toy-task+toy-calc")
	require.Contains(t, report, "[6, 10] 左闭右闭，共 5 个高度")
	require.Contains(t, report, "toy.derived 写 5 次/10 行；删除 0 次")
	require.Contains(t, report, "被拦下的合计   : 写 5 次/10 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 10 行")
}

// task 报错时：不得写同步指针 / 台账 / 任务高度，也不得落半截派生数据。
// 错误本身用「节点侧历史状态不可用」文本 —— 非 Dry 模式下它会被判为可恢复错误并可登记台账，
// 所以「连一条 SQL 都没下发」是有意义的断言（假连接会记录任何语句）。
func TestTaskErrorWritesNeitherPointerNorLedger(t *testing.T) {
	const from, to = 6, 8

	require.Equal(t, syncer.ErrorKindUnrecoverableState, syncer.ClassifySyncError(stateTreeError()),
		"用例前提：该错误文本在生产会被归类为可登记台账的状态不可用错误")

	rec := &Recorder{}
	db := OpenTestDB(t, rec)
	agg := NewFakeAgg(100000, toyTraces(1))
	adapter := &FakeAdapter{}
	counters := NewCounters("toy.derived")
	sink := &toySink{counters: counters}
	// 第一个高度第一次执行时故意报错：走「失败 → 回滚 → 重试」路径
	target := Target{Name: "toy", Groups: []syncer.TaskGroup{{
		toyTask{sink: sink, failEpoch: from, failErr: stateTreeError()},
	}}}

	s, _, err := Assemble(RunOptions{
		From: from, To: to, NoWrite: true,
		EpochsChunk: 1, EpochsThreshold: 3, ErrorWait: time.Millisecond,
	}, target, db, agg, adapter)
	require.NoError(t, err)
	require.NoError(t, s.Init())

	RunSyncerAndWait(t, s.Run, 60*time.Second)

	require.Equal(t, 1, sink.Failures(), "确实发生过一次 task 报错")
	require.Equal(t, int64(3), counters.Snapshot().Calls, "报错高度重试后仍应跑完三个高度")
	require.Empty(t, rec.Statements(), "task 报错时不得向数据库下发任何语句（指针/台账/任务高度/派生表）")
	_, _, rollbacks := rec.TxCounts()
	require.GreaterOrEqual(t, rollbacks, 1, "失败批次应回滚事务，而不是删数据")
}

func TestFormatReport(t *testing.T) {
	t.Run("只统计不落库", func(t *testing.T) {
		got := formatReport(reportInput{
			From: 6357058, To: 6408647, NoWrite: true, Syncer: "evm-contract", Tasks: []string{"evm-transfer-task"},
			Elapsed: 3*time.Second + 250*time.Millisecond,
			Agg:     AggStats{Traces: 514, Tipsets: 1028, ParentTipsets: 514, LatestTipsets: 1},
			Writes: []WriteStat{
				{Table: "fevm.evm_transfers", Calls: 4000, Rows: 9100},
				{Table: "fevm.evm_transfer_stats", Calls: 43, Rows: 1200},
			},
		})
		require.Contains(t, got, "同步器/任务     : evm-contract / evm-transfer-task")
		require.Contains(t, got, "高度区间       : [6357058, 6408647] 左闭右闭，共 51590 个高度")
		require.Contains(t, got, "耗时           : 3.25s")
		require.Contains(t, got, "合计=2057")
		require.Contains(t, got, "--no-write 只统计不落库")
		require.Contains(t, got, "fevm.evm_transfers 写 4000 次/9100 行；删除 0 次")
		require.Contains(t, got, "fevm.evm_transfer_stats 写 43 次/1200 行；删除 0 次")
		require.Contains(t, got, "被拦下的合计   : 写 4043 次/10300 行；删除 0 次")
		require.Contains(t, got, "预计写入行数   : 真写模式下即为 10300 行（本次未落库）")
	})
	t.Run("真写", func(t *testing.T) {
		got := formatReport(reportInput{From: 1, To: 2, NoWrite: false, Syncer: "erc20", Tasks: []string{"erc20-task"}})
		require.Contains(t, got, "模式           : 真写")
		require.Contains(t, got, "已直接下发数据库，未计数")
		require.NotContains(t, got, "预计写入行数")
	})
}

func TestCountersAndSumWrites(t *testing.T) {
	a := NewCounters("t.a")
	b := NewCounters("t.b")
	a.CountWrite(3)
	a.CountWrite(0)
	a.CountDelete()
	b.CountWrite(5)

	require.Equal(t, WriteStat{Table: "t.a", Calls: 2, Rows: 3, Deletes: 1}, a.Snapshot())
	calls, rows, deletes := SumWrites([]WriteStat{a.Snapshot(), b.Snapshot()})
	require.Equal(t, int64(3), calls)
	require.Equal(t, int64(8), rows)
	require.Equal(t, int64(1), deletes)

	// nil 安全：未启用 --no-write 时包装层不存在
	var nilCounters *Counters
	nilCounters.CountWrite(1)
	nilCounters.CountDelete()
	require.Equal(t, WriteStat{}, nilCounters.Snapshot())
}

func TestCountingAggCountsCalls(t *testing.T) {
	agg := NewFakeAgg(100000, nil)
	counted := NewCountingAgg(agg)
	require.Equal(t, agg, counted.Inner())

	ctx := context.Background()
	_, err := counted.LatestTipset(ctx)
	require.NoError(t, err)
	_, err = counted.ParentTipset(ctx, chain.Epoch(6))
	require.NoError(t, err)
	_, err = counted.Tipset(ctx, chain.Epoch(6))
	require.NoError(t, err)
	_, err = counted.Traces(ctx, chain.Epoch(6), chain.Epoch(7))
	require.NoError(t, err)

	require.Equal(t, AggStats{Traces: 1, Tipsets: 1, ParentTipsets: 1, LatestTipsets: 1}, counted.Stats())
	require.Equal(t, int64(4), counted.Stats().Total())
	require.Equal(t, []int64{6}, agg.EpochsRequested())
	require.Panics(t, func() { NewCountingAgg(nil) })
}
