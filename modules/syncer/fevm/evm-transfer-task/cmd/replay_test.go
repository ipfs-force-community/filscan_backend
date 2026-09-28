package evmtransfercmd

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// stateTreeError 与真实生产中「缺口高度在节点侧读不到历史状态」的错误同形
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
		{name: "正常区间", start: 6357058, end: 6408647,
			want: Range{From: 6357058, To: 6408647}},
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

func TestBuildSyncerRejectsInvalidInputs(t *testing.T) {
	conn := &fakeConn{}
	db := newTestGormDB(t, conn)
	agg := newFakeAgg(100000, nil)
	adapter := &fakeAdapter{state: fakeActorState()}

	t.Run("区间非法", func(t *testing.T) {
		_, _, err := BuildSyncer(RunOptions{From: 10, To: 5}, db, agg, adapter, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "不能小于起始高度")
	})
	t.Run("缺数据库", func(t *testing.T) {
		_, _, err := BuildSyncer(RunOptions{From: 1, To: 2}, nil, agg, adapter, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "数据库连接")
	})
	t.Run("缺聚合器", func(t *testing.T) {
		_, _, err := BuildSyncer(RunOptions{From: 1, To: 2}, db, nil, adapter, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "聚合器")
	})
	t.Run("缺适配器", func(t *testing.T) {
		_, _, err := BuildSyncer(RunOptions{From: 1, To: 2}, db, agg, nil, nil)
		require.Error(t, err)
		require.Contains(t, err.Error(), "适配器")
	})
	t.Run("参数齐全时默认值可回退", func(t *testing.T) {
		s, tel, err := BuildSyncer(RunOptions{From: 1, To: 2}, db, agg, adapter, nil)
		require.NoError(t, err) // EpochsChunk/Threshold/ErrorWait 为 0 时回退默认值，不报错
		require.NotNil(t, s)
		require.False(t, tel.NoWrite)
		require.Nil(t, tel.Repo, "真写模式下不做写入计数")
		require.NoError(t, s.Init())
	})
}

// 离线回放主用例：Dry + --no-write 跑完整条 task 管线，要求
//   - 一条 SQL 都不下发（同步指针 / 任务高度 / 台账 / 派生表全部未写）；
//   - 假 repo（派生表写入目标）写入次数 = 0；
//   - 但管线确实跑满 [from, to]（每个高度一个事务），且被拦下的写入被如实统计。
func TestDryNoWritePipelineWritesNothingButCounts(t *testing.T) {
	const from, to = 6, 10

	conn := &fakeConn{}
	db := newTestGormDB(t, conn)
	inner := &fakeRepo{}
	agg := newFakeAgg(100000, func(epoch chain.Epoch) []*londobell.TraceMessage {
		return fakeEvmTraces(epoch, 2)
	})
	adapter := &fakeAdapter{state: fakeActorState()}

	s, tel, err := BuildSyncer(RunOptions{
		From: from, To: to, NoWrite: true,
		EpochsChunk: 2, EpochsThreshold: 5, ErrorWait: time.Millisecond,
	}, db, agg, adapter, inner)
	require.NoError(t, err)
	require.True(t, tel.NoWrite)
	require.NotNil(t, tel.Repo)
	require.NoError(t, s.Init())

	runSyncerAndWait(t, s.Run, 60*time.Second)

	// ① 数据库侧：一条 SQL 都没发（指针/台账/任务高度/派生表都没写）
	require.Empty(t, conn.Statements(), "离线回放不允许向数据库下发任何语句")
	// ② 但管线真的跑完了：每个高度一个事务，且全部提交（本轮无失败批次）
	begins, commits, rollbacks := conn.TxCounts()
	require.Equal(t, 5, begins)
	require.Equal(t, 5, commits)
	require.Zero(t, rollbacks)

	// ③ 假 repo（派生表真写目标）写入次数与行数都为 0
	calls, rows := inner.Writes()
	require.Zero(t, calls, "假 repo 的写入次数必须为 0")
	require.Zero(t, rows)
	transferCalls, transferRows := inner.TransferWrites()
	require.Zero(t, transferCalls)
	require.Zero(t, transferRows)

	// ④ 区间边界：恰好 [from, to] 左闭右闭，不多跑一个高度
	epochs := agg.EpochsRequested()
	sort.Slice(epochs, func(i, j int) bool { return epochs[i] < epochs[j] })
	require.Equal(t, []int64{from, from + 1, from + 2, from + 3, to}, epochs)

	// ⑤ 被拦下的写入 = 真写模式下会落的行（每个高度 2 条 trace → 2 行 evm_transfers）
	ws := tel.Repo.Stats()
	require.Equal(t, int64(5), ws.TransferCalls)
	require.Equal(t, int64(10), ws.TransferRows)
	require.Zero(t, ws.StatCalls, "6..10 不是 120 的整数倍，不触发 1h 聚合统计分支")
	require.Zero(t, ws.DeleteCalls)
	require.Equal(t, int64(10), ws.TotalRows())

	// ⑥ 聚合器调用计量（每高度：Traces 1 次 + ParentTipset 1 次 + Tipset 2 次；每轮 run() 1 次 LatestTipset）
	as := tel.Agg.Stats()
	require.Equal(t, AggStats{Traces: 5, Tipsets: 10, ParentTipsets: 5, LatestTipsets: 1}, as)
	require.Equal(t, int64(21), as.Total())

	// ⑦ 报告能读到区间、模式与预计写入行数
	report := tel.Report(Range{From: from, To: to}, 2*time.Second)
	require.Contains(t, report, "[6, 10] 左闭右闭，共 5 个高度")
	require.Contains(t, report, "--no-write")
	require.Contains(t, report, "聚合器调用     : Traces=5 Tipset=10 ParentTipset=5 LatestTipset=1 合计=21")
	require.Contains(t, report, "fevm.evm_transfers 5 次/10 行")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 10 行")
}

// 对照组：同一份配置去掉 --no-write 后，派生表写入会真的走到「假 repo」上
// （证明用例③的「零写入」不是假象，而是 no-write 起了作用）。
func TestWriteModeDoesReachDerivedRepo(t *testing.T) {
	const from, to = 6, 7

	conn := &fakeConn{}
	db := newTestGormDB(t, conn)
	inner := &fakeRepo{}
	agg := newFakeAgg(100000, func(epoch chain.Epoch) []*londobell.TraceMessage {
		return fakeEvmTraces(epoch, 2)
	})
	adapter := &fakeAdapter{state: fakeActorState()}

	s, tel, err := BuildSyncer(RunOptions{
		From: from, To: to, NoWrite: false,
		EpochsChunk: 1, EpochsThreshold: 2, ErrorWait: time.Millisecond,
	}, db, agg, adapter, inner)
	require.NoError(t, err)
	require.Nil(t, tel.Repo)
	require.NoError(t, s.Init())

	runSyncerAndWait(t, s.Run, 60*time.Second)

	transferCalls, transferRows := inner.TransferWrites()
	require.Equal(t, 2, transferCalls) // 两个高度各一次
	require.Equal(t, 4, transferRows)
	// 即使是真写模式，Dry 仍然保证不写指针/台账/任务高度：一条 SQL 都没有
	require.Empty(t, conn.Statements())
}

// task 报错时：不得写同步指针（chain.sync_syncers）、不得写跳过台账（chain.sync_skipped_epochs）、
// 也不得落半截派生数据。错误本身用的是「节点侧历史状态不可用」文本 —— 这在非 Dry 模式下
// 是会被判为不可恢复并可能登记台账的那一类，因此「没写台账」是一条有意义的断言。
func TestTaskErrorWritesNeitherPointerNorLedger(t *testing.T) {
	const from, to = 6, 8

	require.Equal(t, syncer.ErrorKindUnrecoverableState, syncer.ClassifySyncError(stateTreeError()),
		"用例前提：该错误在非 Dry 模式下会被归类为可登记台账的不可恢复错误")

	conn := &fakeConn{}
	db := newTestGormDB(t, conn)
	inner := &fakeRepo{}
	agg := newFakeAgg(100000, func(epoch chain.Epoch) []*londobell.TraceMessage {
		return fakeEvmTraces(epoch, 1)
	})
	adapter := &fakeAdapter{state: fakeActorState(), errOnce: stateTreeError()}

	// 在「首次报错」的那一刻快照：SQL 与派生表写入都必须仍是 0
	var atErrSQL, atErrWriteCalls, atErrWriteRows int
	adapter.onError = func() {
		atErrSQL = len(conn.Statements())
		atErrWriteCalls, atErrWriteRows = inner.Writes()
	}

	s, tel, err := BuildSyncer(RunOptions{
		From: from, To: to, NoWrite: true,
		EpochsChunk: 1, EpochsThreshold: 3, ErrorWait: time.Millisecond,
	}, db, agg, adapter, inner)
	require.NoError(t, err)
	require.NotNil(t, tel.Repo)
	require.NoError(t, s.Init())

	runSyncerAndWait(t, s.Run, 60*time.Second)

	calls, errs := adapter.Calls()
	require.Equal(t, 1, errs, "确实发生过一次 task 报错")
	require.GreaterOrEqual(t, calls, 2, "报错高度被重试并最终跑通")

	// ① 报错那一刻：没有指针、没有台账、没有派生表写入
	require.Zero(t, atErrSQL, "task 报错时不得向数据库下发任何语句（仅回滚）")
	require.Zero(t, atErrWriteCalls)
	require.Zero(t, atErrWriteRows)

	// ② 整轮跑完：依然一条 SQL 都没发
	require.Empty(t, conn.Statements(), "指针/台账/任务高度在整个回放过程中都不得被写")
	c, r := inner.Writes()
	require.Zero(t, c, "派生表写入次数必须为 0")
	require.Zero(t, r)
	// ③ 回滚走的是事务回滚，而不是 Delete（Delete 会真的删数据）
	_, _, rollbacks := conn.TxCounts()
	require.GreaterOrEqual(t, rollbacks, 1, "失败批次应回滚事务")
	require.Zero(t, tel.Repo.Stats().DeleteCalls)
}

func TestFormatReport(t *testing.T) {
	t.Run("只统计不落库", func(t *testing.T) {
		got := formatReport(reportInput{
			From: 6357058, To: 6408647, NoWrite: true,
			Elapsed: 3*time.Second + 250*time.Millisecond,
			Agg:     AggStats{Traces: 514, Tipsets: 1028, ParentTipsets: 514, LatestTipsets: 1},
			Writes:  NoWriteStats{TransferCalls: 4000, TransferRows: 9100, StatCalls: 43, StatRows: 1200, DeleteCalls: 0},
		})
		require.Contains(t, got, "同步器/任务     : evm-contract / evm-transfer-task")
		require.Contains(t, got, "高度区间       : [6357058, 6408647] 左闭右闭，共 51590 个高度")
		require.Contains(t, got, "耗时           : 3.25s")
		require.Contains(t, got, "合计=2057")
		require.Contains(t, got, "--no-write 只统计不落库")
		require.Contains(t, got, "fevm.evm_transfers 4000 次/9100 行；fevm.evm_transfer_stats 43 次/1200 行；删除 0 次；合计 10300 行")
		require.Contains(t, got, "预计写入行数   : 真写模式下即为 10300 行（本次未落库）")
	})
	t.Run("真写", func(t *testing.T) {
		got := formatReport(reportInput{From: 1, To: 2, NoWrite: false, Elapsed: time.Second})
		require.Contains(t, got, "模式           : 真写")
		require.Contains(t, got, "已直接下发数据库，未计数")
		require.NotContains(t, got, "预计写入行数")
	})
}

func TestDerefInt64(t *testing.T) {
	require.Zero(t, derefInt64(nil))
	v := int64(7)
	require.Equal(t, int64(7), derefInt64(&v))
}

func TestConfLine(t *testing.T) {
	require.Contains(t, confLine(nil), "<未配置>")
	require.Contains(t, confLine(&config.Config{}), "<未配置>")

	agg, adapter := "http://172.31.38.30:12345", "http://172.31.38.30:12346"
	got := confLine(&config.Config{Londobell: &config.Londobell{AggAddress: &agg, AdapterAddress: &adapter}})
	require.Contains(t, got, "聚合器=http://172.31.38.30:12345")
	require.Contains(t, got, "适配器=http://172.31.38.30:12346")
	require.NotContains(t, got, "postgres://", "不得把 DSN 打进日志")
}
