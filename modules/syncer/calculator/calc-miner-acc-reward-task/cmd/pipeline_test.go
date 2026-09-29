package mineraccrewardcmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

// 本文件用**真实的 CalcMinerAccRewardTask** 跑完整条 Dry 清单管线，是
// 「miner-acc-reward 子命令真能补 chain.miner_reward_stats、只写派生表、--no-write 下零写入、
// 输入（chain.miner_rewards / chain.builtin_actor_states）被真的读到了」的主证据。

const (
	accHeadEpoch = 6409045 // 假聚合器链头（head-1 才是可回放的最高高度）
	accReplayA   = 6357122
	accReplayB   = 6357123
)

// 四个 interval 的区间长度（与 calc-miner-acc-reward-task.go:97-110 一致）
var intervals = map[string]int64{
	"24h": 2880,
	"7d":  2880 * 7,
	"30d": 2880 * 30,
	"1y":  2880 * 365,
}

// accReplay 一次回放的装配结果
type accReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *offlinereplay.FakeAgg
	adapter *offlinereplay.FakeAdapter
	inner   *fakeRewardRepo
}

// buildAccReplay 用真实计算器 + 假仓储/假聚合器/假适配器 + 假连接（不下发任何 SQL）装配清单回放。
func buildAccReplay(t *testing.T, noWrite bool, heights ...int64) *accReplay {
	t.Helper()

	inner := &fakeRewardRepo{}
	restore := swapRewardRepo(inner)
	t.Cleanup(restore)

	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)

	target, writes := buildTarget(db, noWrite)

	text := ""
	for _, h := range heights {
		text += fmt.Sprintf("%d\n", h)
	}
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	plan, err := offlinereplay.ResolvePlan(0, 0, path)
	require.NoError(t, err)

	return &accReplay{
		rec: rec, db: db, plan: plan, target: target, writes: writes,
		agg: offlinereplay.NewFakeAgg(accHeadEpoch, nil), adapter: &offlinereplay.FakeAdapter{}, inner: inner,
	}
}

// execute 跑一次回放并返回打印出来的报告
func (r *accReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureAccStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

func captureAccStdout(t *testing.T, fn func()) string {
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

// --no-write：真实计算器跑满清单，chain.miner_reward_stats 的写入只被计数、一次都没转发到真仓储，
// 且一条 SQL 都没下发（同步指针 / 台账 / 任务高度都不写）；也不该有任何 Traces 调用。
func TestMinerAccRewardNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rp := buildAccReplay(t, true, accReplayA, accReplayB)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
	require.Empty(t, rp.agg.EpochsRequested(), "本计算器不读 traces ⇒ 不该有 Traces 调用")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range rp.writes() {
		stats[s.Table] = s
	}
	// 每高度 4 行（24h/7d/30d/1y）
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewardStats, Calls: 2, Rows: 8}, stats[TableMinerRewardStats])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewards}, stats[TableMinerRewards],
		"本命令不跑 reward-task ⇒ 不碰 chain.miner_rewards")

	require.Contains(t, report, "同步器/任务     : chain / calc-miner-acc-reward-task")
	require.Contains(t, report, "2 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "chain.miner_reward_stats 写 2 次/8 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 8 行")
}

// 真写：每高度 4 行，interval 齐全、epoch 就是清单高度；且四个区间求和与全网算力都真的读了
// （这两个输入分别来自 chain.miner_rewards 与 chain.builtin_actor_states —— 空洞里都为空）。
func TestMinerAccRewardDryModeWritesFourIntervals(t *testing.T) {
	rp := buildAccReplay(t, false, accReplayA, accReplayB)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Equal(t, 2, len(rp.inner.rewardStats), "每个高度一次 SaveMinerRewardStats")

	for i, rows := range rp.inner.rewardStats {
		wantEpoch := []int64{accReplayA, accReplayB}[i]
		require.Len(t, rows, 4)
		got := map[string]bool{}
		for _, r := range rows {
			require.Equal(t, wantEpoch, r.Epoch, "写入的高度必须就是清单里的高度")
			got[r.Interval] = true
			// AccRewardPerT = reward / (power / PerT) = 1000 / 10 = 100
			require.Equal(t, 0, r.AccRewardPerT.Cmp(decimal.NewFromInt(100)))
		}
		require.Equal(t, map[string]bool{"24h": true, "7d": true, "30d": true, "1y": true}, got)
	}

	// 读入区间：每高度 4 次 SumRewards（左闭右闭，起点 = 高度 - interval，终点 = 高度）
	require.Len(t, rp.inner.sumRanges, 8)
	for i, h := range []int64{accReplayA, accReplayB} {
		got := map[int64]bool{}
		for _, r := range rp.inner.sumRanges[i*4 : i*4+4] {
			require.Equal(t, h, r.LteEnd.Int64(), "求和终点必须是当前高度")
			got[h-r.GteBegin.Int64()] = true
		}
		want := map[int64]bool{}
		for _, d := range intervals {
			want[d] = true
		}
		require.Equal(t, want, got, "四个区间分别是 24h/7d/30d/1y（输入是 chain.miner_rewards）")
	}

	// 全网算力：每高度 4 次（每个 interval 各一次），高度取当前高度（输入是 chain.builtin_actor_states）
	require.Len(t, rp.inner.powerEpoch, 8)
	require.Equal(t, int64(accReplayA), rp.inner.powerEpoch[0].Int64())
	require.Equal(t, int64(accReplayB), rp.inner.powerEpoch[4].Int64())

	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// 输入为空的样子（这正是必须先跑 miner-rewards / baseline-actors 的原因）：
// chain.miner_rewards 为空 ⇒ AccReward = 0；chain.builtin_actor_states 为空 ⇒ power = 0 ⇒ AccRewardPerT = 0。
func TestMinerAccRewardEmptyInputsYieldZeroColumns(t *testing.T) {
	rp := buildAccReplay(t, false, accReplayA)
	rp.inner.emptyInputs = true
	rp.execute(t, offlinereplay.RunOptions{})

	require.Len(t, rp.inner.rewardStats, 1)
	for _, row := range rp.inner.rewardStats[0] {
		require.True(t, row.AccReward.IsZero(), "miner_rewards 为空 ⇒ acc_reward = 0")
		require.True(t, row.AccRewardPerT.IsZero(), "builtin_actor_states 为空 ⇒ acc_reward_per_t = 0")
	}
}

// 高于链头的高度在开跑前被拦下（清单模式不会去等未来高度，避免整批卡死）
func TestMinerAccRewardRejectsHeightsAboveHead(t *testing.T) {
	rp := buildAccReplay(t, false, accReplayA, accHeadEpoch+1)

	err := offlinereplay.Execute(rp.plan, offlinereplay.RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		rp.target, rp.db, rp.agg, rp.adapter, rp.writes)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("高于聚合器链头 %d", accHeadEpoch-1))
	require.Zero(t, rp.inner.Writes())
	require.Empty(t, rp.rec.Statements())
}
