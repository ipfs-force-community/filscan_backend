package minerrewardscmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

// newOwnerReward 造一条「上一条 owner_rewards」（累计口径的起点）
func newOwnerReward(epoch, accReward, accBlockCount int64) *owner.Reward {
	return &owner.Reward{
		Epoch:         chain.Epoch(epoch),
		Owner:         "t0200",
		AccReward:     chain.AttoFil(decimal.NewFromInt(accReward)),
		AccBlockCount: accBlockCount,
	}
}

// 本文件用**真实的 MinerRewardTask** 跑完整条 Dry 管线，是「miner-rewards 子命令真能补
// chain.miner_rewards / chain.owner_rewards / chain.miner_win_counts、只写派生表、
// --no-write 下零写入」的主证据。

const (
	headEpoch = 6409045 // 假聚合器链头（head-1 才是可回放的最高高度）
	replayA   = 6357122
	replayB   = 6357123
)

// fakeMinerAgg 在框架假聚合器上加两个本链路要用的方法（MinersBlockReward / WinCount）。
// 其余方法由框架假实现（或嵌入式 nil 接口）兜底：未实现的方法一旦被调用即会暴露出来。
type fakeMinerAgg struct {
	*offlinereplay.FakeAgg

	// onlyMiner 非空时只返回这一个矿工的爆块奖励与赢票（用于把「同高度一个 owner 下只有一个爆块矿工」
	// 的语义单独隔出来验证）
	onlyMiner string
}

func (f *fakeMinerAgg) MinersBlockReward(_ context.Context, start, _ chain.Epoch) ([]*londobell.MinersBlockReward, error) {
	if f.onlyMiner != "" {
		return []*londobell.MinersBlockReward{
			{Id: londobell.EpochMiner{Epoch: start.Int64(), Miner: f.onlyMiner}, TotalBlockReward: decimal.NewFromInt(100), BlockCount: 2},
		}, nil
	}
	return []*londobell.MinersBlockReward{
		{Id: londobell.EpochMiner{Epoch: start.Int64(), Miner: "t0100"}, TotalBlockReward: decimal.NewFromInt(100), BlockCount: 2},
		{Id: londobell.EpochMiner{Epoch: start.Int64(), Miner: "t0101"}, TotalBlockReward: decimal.NewFromInt(200), BlockCount: 3},
	}, nil
}

func (f *fakeMinerAgg) WinCount(_ context.Context, _, _ chain.Epoch) ([]*londobell.MinerWinCount, error) {
	if f.onlyMiner != "" {
		return []*londobell.MinerWinCount{{Id: f.onlyMiner, TotalWinCount: 5}}, nil
	}
	return []*londobell.MinerWinCount{
		{Id: "t0100", TotalWinCount: 5},
		{Id: "t0101", TotalWinCount: 7},
	}, nil
}

// fakeMinerAdapter 只实现 reward-task 用到的 adapter.Miner（取矿工的 owner）。
// 两个矿工故意挂在**同一个 owner** 下，用来验证 owner_rewards 的按 owner 聚合。
type fakeMinerAdapter struct {
	londobell.Adapter
}

func (f *fakeMinerAdapter) Miner(_ context.Context, miner chain.SmartAddress, _ *chain.Epoch) (*londobell.MinerDetail, error) {
	return &londobell.MinerDetail{Miner: miner.Address(), Owner: "t0200"}, nil
}

// rewardReplay 一次回放的装配结果
type rewardReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *fakeMinerAgg
	adapter *fakeMinerAdapter
	inner   *fakeRewardRepo
}

// buildRewardReplay 用真实任务 + 假仓储/假聚合器/假适配器 + 假连接（不下发任何 SQL）装配清单回放。
func buildRewardReplay(t *testing.T, noWrite bool, heights ...int64) *rewardReplay {
	t.Helper()
	return buildRewardReplayWithAgg(t, noWrite,
		&fakeMinerAgg{FakeAgg: offlinereplay.NewFakeAgg(headEpoch, nil)}, heights...)
}

// buildRewardReplayWithAgg 同上，但允许指定聚合器（例如只返回一个爆块矿工）
func buildRewardReplayWithAgg(t *testing.T, noWrite bool, agg *fakeMinerAgg, heights ...int64) *rewardReplay {
	t.Helper()

	inner := &fakeRewardRepo{}
	restore := swapRepo(inner)
	t.Cleanup(restore)

	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)
	adapter := &fakeMinerAdapter{}

	target, writes := buildTarget(db, noWrite)

	text := ""
	for _, h := range heights {
		text += fmt.Sprintf("%d\n", h)
	}
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	plan, err := offlinereplay.ResolvePlan(0, 0, path)
	require.NoError(t, err)

	return &rewardReplay{rec: rec, db: db, plan: plan, target: target, writes: writes, agg: agg, adapter: adapter, inner: inner}
}

// execute 跑一次回放并返回打印出来的报告
func (r *rewardReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

// captureStdout 抓取 fn 期间写到 os.Stdout 的内容（Execute 用 fmt.Println 打印报告）
func captureStdout(t *testing.T, fn func()) string {
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

// --no-write：真实任务跑满清单，三张派生表的写入只被计数、一次都没转发到真仓储，
// 且一条 SQL 都没下发（同步指针 / 台账 / 任务高度都不写）。
func TestMinerRewardsNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rp := buildRewardReplay(t, true, replayA, replayB)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
	require.Empty(t, rp.agg.EpochsRequested(),
		"reward-task 不读 traces ⇒ 不该有 Traces 调用")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range rp.writes() {
		stats[s.Table] = s
	}
	// 每高度：2 个矿工奖励行、1 个 owner（两个矿工同 owner 聚合）、2 个赢票行
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewards, Calls: 2, Rows: 4}, stats[TableMinerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerRewards, Calls: 2, Rows: 2}, stats[TableOwnerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerWinCounts, Calls: 2, Rows: 4}, stats[TableMinerWinCounts])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewardStats}, stats[TableMinerRewardStats],
		"本命令不跑 calc-miner-acc-reward-task ⇒ 不碰 chain.miner_reward_stats")
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerAggRewards}, stats[TableMinerAggRewards],
		"本命令不跑 calc-miner-agg-reward ⇒ 不碰 chain.miner_agg_rewards")

	require.Contains(t, report, "同步器/任务     : chain / reward-task")
	require.Contains(t, report, "2 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "chain.miner_rewards 写 2 次/4 行；删除 0 次")
	require.Contains(t, report, "chain.miner_win_counts 写 2 次/4 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 10 行")
}

// 不加 --no-write（真写通道 = 假仓储）时：三张派生表真写入，且写入的高度就是清单里的高度；
// Dry 依然保证不写同步指针 / 台账 / 任务高度（一条 SQL 都没有）。
func TestMinerRewardsDryModeWritesDerivedTablesOnly(t *testing.T) {
	rp := buildRewardReplay(t, false, replayA, replayB)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Equal(t, 2, len(rp.inner.minerRewards), "每个高度一次 SaveMinerRewards")
	require.Equal(t, 2, len(rp.inner.ownerRewards))
	require.Equal(t, 2, len(rp.inner.winCounts))
	require.Empty(t, rp.inner.rewardStats, "不写 chain.miner_reward_stats")
	require.Empty(t, rp.inner.aggRewards, "不写 chain.miner_agg_rewards")

	for i, rewards := range rp.inner.minerRewards {
		wantEpoch := []int64{replayA, replayB}[i]
		require.Len(t, rewards, 2)
		for _, r := range rewards {
			require.Equal(t, wantEpoch, r.Epoch.Int64(), "写入的高度必须就是清单里的高度")
		}
	}

	// owner_rewards 按 owner 聚合：两个矿工同属 t0200 ⇒ 该高度只有一行，奖励与爆块数相加
	for i, owners := range rp.inner.ownerRewards {
		wantEpoch := []int64{replayA, replayB}[i]
		require.Len(t, owners, 1)
		require.Equal(t, wantEpoch, owners[0].Epoch.Int64())
		require.Equal(t, "t0200", owners[0].Owner.Address())
		require.Equal(t, 0, owners[0].Reward.Decimal().Cmp(decimal.NewFromInt(300)))
		require.Equal(t, int64(5), owners[0].BlockCount)
		require.Len(t, owners[0].Miners, 2)
	}

	for i, counts := range rp.inner.winCounts {
		wantEpoch := []int64{replayA, replayB}[i]
		require.Len(t, counts, 2)
		for _, c := range counts {
			require.Equal(t, wantEpoch, c.Epoch, "写入的高度必须就是清单里的高度")
		}
	}

	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// owner_rewards 的累计口径来自「同一 owner 的上一条记录」：查得到时 acc_* 接着累计，
// prev_epoch_ref 指向上一条；查不到（如没补前面的高度）时从零开始 —— 这正是必须按升序连续补的原因。
func TestMinerRewardsAccumulatesFromPreviousOwnerReward(t *testing.T) {
	// 该高度该 owner 下只有一个爆块矿工（把「上一条只加一次」的语义单独隔离出来）
	agg := &fakeMinerAgg{FakeAgg: offlinereplay.NewFakeAgg(headEpoch, nil), onlyMiner: "t0100"}
	rp := buildRewardReplayWithAgg(t, false, agg, replayA)

	rp.inner.lastOwnerReward = newOwnerReward(replayA-120, 1000, 8)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Len(t, rp.inner.ownerRewards, 1)
	got := rp.inner.ownerRewards[0][0]
	require.Equal(t, int64(replayA-120), got.PrevEpochRef.Int64(), "prev_epoch_ref 指向上一条 owner_rewards")
	require.Equal(t, 0, got.AccReward.Decimal().Cmp(decimal.NewFromInt(1100)), "acc_reward = 本高度 100 + 上一条 1000")
	require.Equal(t, int64(10), got.AccBlockCount, "acc_block_count = 本高度 2 + 上一条 8")

	// 无上一条记录时：acc_* 从零开始（只是「这一高度」的值，不是真实累计值）
	rp2 := buildRewardReplayWithAgg(t, false, agg, replayB)
	rp2.execute(t, offlinereplay.RunOptions{})
	require.Equal(t, 0, rp2.inner.ownerRewards[0][0].AccReward.Decimal().Cmp(decimal.NewFromInt(100)))
}

// 生产实现的一个既有偏差（本命令**忠实复现**、未修改；见包注释「已知偏差」）：
// reward_task.go:159-168 把「上一条 owner_rewards」加在**每个爆块矿工**的循环里，
// 于是同一高度同一 owner 下有 N 个爆块矿工时，上一条会被累加 N 次 ⇒ acc_reward / acc_block_count 偏高。
// 这里钉住现状，避免「以为补出来的累计值是准的」。
func TestMinerRewardsPreviousRowIsAddedOncePerMiner(t *testing.T) {
	rp := buildRewardReplay(t, false, replayA) // 两个矿工同属 t0200
	rp.inner.lastOwnerReward = newOwnerReward(replayA-120, 1000, 8)
	rp.execute(t, offlinereplay.RunOptions{})

	got := rp.inner.ownerRewards[0][0]
	require.Equal(t, 0, got.Reward.Decimal().Cmp(decimal.NewFromInt(300)), "本高度奖励本身是对的")
	require.Equal(t, int64(5), got.BlockCount, "本高度爆块数本身是对的")
	require.Equal(t, 0, got.AccReward.Decimal().Cmp(decimal.NewFromInt(2300)),
		"现状：300 + 上一条 1000 × 2 个矿工（偏高，属既有实现偏差，不在本命令范围内）")
	require.Equal(t, int64(21), got.AccBlockCount, "现状：5 + 8 × 2")
}

// 高于链头的高度在开跑前被拦下（清单模式不会去等未来高度，避免整批卡死）
func TestMinerRewardsRejectsHeightsAboveHead(t *testing.T) {
	rp := buildRewardReplay(t, false, replayA, headEpoch+1)

	err := offlinereplay.Execute(rp.plan, offlinereplay.RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		rp.target, rp.db, rp.agg, rp.adapter, rp.writes)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("高于聚合器链头 %d", headEpoch-1))
	require.Zero(t, rp.inner.Writes())
	require.Empty(t, rp.rec.Statements())
}
