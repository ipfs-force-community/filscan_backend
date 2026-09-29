package mineraggrewardcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

// 本文件用**真实的 CalcMinerAggReward** 跑整条 Dry 区间管线，是
// 「miner-agg-reward 子命令真能刷新 chain.miner_agg_rewards、只写派生表、--no-write 下零写入、
// 只在批内最后一个高度触发一次聚合」的主证据。

const (
	aggHeadEpoch = 6409045 // 假聚合器链头（head-1 才是可回放的最高高度）
	aggFrom      = 6357122
	aggTo        = 6357126 // 5 个高度：小于 epochsThreshold（20）⇒ 整段一批
)

// aggReplay 一次回放的装配结果
type aggReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *offlinereplay.FakeAgg
	adapter *offlinereplay.FakeAdapter
	inner   *fakeRewardRepo
}

// buildAggReplay 用真实计算器 + 假仓储/假聚合器/假适配器 + 假连接（不下发任何 SQL）装配**区间**回放。
func buildAggReplay(t *testing.T, noWrite bool) *aggReplay {
	t.Helper()

	inner := &fakeRewardRepo{}
	restore := swapRewardRepo(inner)
	t.Cleanup(restore)

	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)

	target, writes := buildTarget(db, noWrite)

	plan, err := offlinereplay.ResolvePlan(aggFrom, aggTo, "")
	require.NoError(t, err)
	require.False(t, plan.IsList(), "本命令只支持区间模式")
	require.NoError(t, validatePlan(plan))

	return &aggReplay{
		rec: rec, db: db, plan: plan, target: target, writes: writes,
		agg: offlinereplay.NewFakeAgg(aggHeadEpoch, nil), adapter: &offlinereplay.FakeAdapter{}, inner: inner,
	}
}

// execute 跑一次回放并返回打印出来的报告
func (r *aggReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureAggStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

func captureAggStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	rd, wr, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = wr
	defer func() { os.Stdout = old }()

	fn()
	require.NoError(t, wr.Close())
	out, err := io.ReadAll(rd)
	require.NoError(t, err)
	return string(out)
}

// --no-write：真实计算器跑完整段，chain.miner_agg_rewards 的写入只被计数、一次都没转发到真仓储，
// 且一条 SQL 都没下发（同步指针 / 台账 / 任务高度都不写）；也不该有任何 Traces 调用。
func TestMinerAggRewardNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rp := buildAggReplay(t, true)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
	require.Empty(t, rp.agg.EpochsRequested(), "本计算器不读 traces ⇒ 不该有 Traces 调用")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range rp.writes() {
		stats[s.Table] = s
	}
	// 整段一批 ⇒ 只在最后一个高度触发一次聚合，两个矿工各一行
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerAggRewards, Calls: 1, Rows: 2}, stats[TableMinerAggRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewards}, stats[TableMinerRewards],
		"本命令不跑 reward-task ⇒ 不碰 chain.miner_rewards")

	require.Contains(t, report, "同步器/任务     : chain / calc-miner-agg-reward")
	require.Contains(t, report, fmt.Sprintf("高度区间       : [%d, %d] 左闭右闭，共 5 个高度", aggFrom, aggTo))
	require.Contains(t, report, "chain.miner_agg_rewards 写 1 次/2 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 2 行")
}

// 真写：整段一批 ⇒ 计算器只在**批内最后一个高度**执行一次（calc-miner-agg-reward.go:35 的 LastCalc 门槛），
// 取矿工用的批区间就是整段 [from, to]（输入是 chain.miner_rewards）。
// 注意：本计算器**不会**为每个高度写行 —— chain.miner_agg_rewards 是「每矿工一条」的全表聚合。
func TestMinerAggRewardRunsOncePerBatchAndWritesEveryMiner(t *testing.T) {
	rp := buildAggReplay(t, false)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Len(t, rp.inner.minerRanges, 1, "5 个高度一段（阈值 20）⇒ 只在一个高度触发一次聚合")
	require.Equal(t, int64(aggFrom), rp.inner.minerRanges[0].GteBegin.Int64())
	require.Equal(t, int64(aggTo), rp.inner.minerRanges[0].LteEnd.Int64())

	require.Len(t, rp.inner.aggRewards, 1)
	require.Len(t, rp.inner.aggRewards[0], 2, "两个矿工各一行（主键 = miner，覆盖式 upsert）")
	miners := map[string]bool{}
	for _, r := range rp.inner.aggRewards[0] {
		miners[r.Miner] = true
	}
	require.Equal(t, map[string]bool{"t0100": true, "t0101": true}, miners)

	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// 造一个真正的清单计划，确认 validatePlan 会拦下它（不静默空跑）
func TestMinerAggRewardRejectsListPlanBeforeConnecting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf("%d\n%d\n", aggFrom, aggTo)), 0o600))

	plan, err := offlinereplay.ResolvePlan(0, 0, path)
	require.NoError(t, err)
	require.True(t, plan.IsList())
	require.Error(t, validatePlan(plan))
}
