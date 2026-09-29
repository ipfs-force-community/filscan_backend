package baselineactorscmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/filecoin-project/go-state-types/builtin"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

// 本文件用**真实的 BaselineTask** 跑完整条 Dry 清单管线，是「baseline-actors 子命令真能补
// chain.builtin_actor_states、只写派生表、--no-write 下零写入、且它的输入只有适配器（无数据库依赖）
// ⇒ 空洞里可安全回放」的主证据。

const (
	baseHeadEpoch = 6409045 // 假聚合器链头（head-1 才是可回放的最高高度）
	baseReplayA   = 6357122
	baseReplayB   = 6357123
)

// builtinActorStates 造两个内置 actor 的状态（baseline-task 会取这两个地址的状态并序列化落库）
func builtinActorStates() map[chain.SmartAddress]*londobell.ActorState {
	return map[chain.SmartAddress]*londobell.ActorState{
		chain.SmartAddress(builtin.RewardActorAddr.String()): {
			ActorID:   builtin.RewardActorAddr.String(),
			ActorAddr: builtin.RewardActorAddr.String(),
			Balance:   decimal.NewFromInt(100),
			State:     map[string]interface{}{"ThisEpochReward": "123"},
		},
		chain.SmartAddress(builtin.StoragePowerActorAddr.String()): {
			ActorID:   builtin.StoragePowerActorAddr.String(),
			ActorAddr: builtin.StoragePowerActorAddr.String(),
			Balance:   decimal.NewFromInt(200),
			State:     map[string]interface{}{"TotalQualityAdjPower": "456", "ThisEpochQualityAdjPower": "450"},
		},
	}
}

// baseReplay 一次回放的装配结果
type baseReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *offlinereplay.FakeAgg
	adapter londobell.Adapter
	inner   *fakeBaselineRepo
}

// buildBaseReplay 用真实任务 + 假仓储/假聚合器/假适配器 + 假连接（不下发任何 SQL）装配清单回放。
func buildBaseReplay(t *testing.T, noWrite bool, heights ...int64) *baseReplay {
	t.Helper()

	inner := &fakeBaselineRepo{}
	restore := restoreBaselineRepo(inner)
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

	return &baseReplay{
		rec: rec, db: db, plan: plan, target: target, writes: writes,
		agg:     offlinereplay.NewFakeAgg(baseHeadEpoch, nil),
		adapter: &offlinereplay.FakeAdapter{StateByActor: builtinActorStates()},
		inner:   inner,
	}
}

// execute 跑一次回放并返回打印出来的报告
func (r *baseReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureBaseStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

func captureBaseStdout(t *testing.T, fn func()) string {
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

// --no-write：真实任务跑满清单，chain.builtin_actor_states 的写入只被计数、一次都没转发到真仓储，
// 且一条 SQL 都没下发（同步指针 / 台账 / 任务高度都不写）；也不该有任何 Traces 调用。
func TestBaselineActorsNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rp := buildBaseReplay(t, true, baseReplayA, baseReplayB)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
	require.Empty(t, rp.agg.EpochsRequested(), "baseline-task 不读 traces ⇒ 不该有 Traces 调用")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range rp.writes() {
		stats[s.Table] = s
	}
	// 每高度 2 行（reward actor + storage power actor）
	require.Equal(t, offlinereplay.WriteStat{Table: TableBuiltinActorStates, Calls: 2, Rows: 4}, stats[TableBuiltinActorStates])

	require.Contains(t, report, "同步器/任务     : chain / baseline-task")
	require.Contains(t, report, "2 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "chain.builtin_actor_states 写 2 次/4 行；删除 0 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 4 行")
}

// 真写：每高度 2 行，actor 就是 reward / storage power 两个内置 actor，epoch 就是清单高度，
// 状态 JSON 落库（下游从 state->>'TotalQualityAdjPower' 取全网算力）。
func TestBaselineActorsDryModeWritesBothBuiltinActors(t *testing.T) {
	rp := buildBaseReplay(t, false, baseReplayA, baseReplayB)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Equal(t, 2, len(rp.inner.saves), "每个高度一次 SaveBuiltActorStates")
	require.Empty(t, rp.inner.deletes, "DeleteBuiltActorStates 只在 RollBack 路径走，正常不该被调用")

	for i, rows := range rp.inner.saves {
		wantEpoch := []int64{baseReplayA, baseReplayB}[i]
		require.Len(t, rows, 2)
		actors := map[string]bool{}
		for _, r := range rows {
			require.Equal(t, wantEpoch, r.Epoch)
			actors[r.Actor] = true
			require.NotEmpty(t, r.State, "状态 JSON 必须落库（下游要 state->>'TotalQualityAdjPower'）")
		}
		require.Equal(t, map[string]bool{
			builtin.RewardActorAddr.String():       true,
			builtin.StoragePowerActorAddr.String(): true,
		}, actors)
	}

	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// 输入只有适配器（节点侧按高度回溯取状态）——没有数据库输入 ⇒ 空洞里可安全回放。
// 适配器报错时该高度失败、被保险丝放弃，其余高度照补。
func TestBaselineActorsAdapterErrorAbandonsOnlyThatHeight(t *testing.T) {
	const bad = baseReplayA + 2 // 与两个健康高度都不同
	rp := buildBaseReplay(t, false, baseReplayA, bad, baseReplayB)
	// 只在 bad 高度让适配器报错：把假适配器换成「按高度报错」的包装
	rp.adapter = &failingAdapter{
		FakeAdapter: &offlinereplay.FakeAdapter{StateByActor: builtinActorStates()},
		failEpoch:   bad,
	}

	report := rp.execute(t, offlinereplay.RunOptions{EpochFailLimit: 2})

	require.Equal(t, 2, len(rp.inner.saves), "只有两个健康高度补上了 chain.builtin_actor_states")
	var patched []int64
	for _, rows := range rp.inner.saves {
		patched = append(patched, rows[0].Epoch)
	}
	require.Equal(t, []int64{baseReplayA, baseReplayB}, patched)

	require.Empty(t, rp.rec.Statements(), "报错也不得写指针/台账/任务高度")
	require.Contains(t, report, "逐高度结果     : 已跑 3/3 个高度；放弃 1 个；未跑 0 个")
	require.Contains(t, report, fmt.Sprintf("- 高度 %d 失败 2 次", bad))
}

// failingAdapter 只在指定高度让 Actor 调用失败（其余高度透传给框架假适配器）
type failingAdapter struct {
	*offlinereplay.FakeAdapter
	failEpoch int64
}

func (f *failingAdapter) Actor(ctx context.Context, id chain.SmartAddress, epoch *chain.Epoch) (*londobell.ActorState, error) {
	if epoch != nil && epoch.Int64() == f.failEpoch {
		return nil, fmt.Errorf("fake: 高度 %d 的内置 actor 状态取不到", f.failEpoch)
	}
	return f.FakeAdapter.Actor(ctx, id, epoch)
}

// 高于链头的高度在开跑前被拦下（清单模式不会去等未来高度，避免整批卡死）
func TestBaselineActorsRejectsHeightsAboveHead(t *testing.T) {
	rp := buildBaseReplay(t, false, baseReplayA, baseHeadEpoch+1)

	err := offlinereplay.Execute(rp.plan, offlinereplay.RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		rp.target, rp.db, rp.agg, rp.adapter, rp.writes)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("高于聚合器链头 %d", baseHeadEpoch-1))
	require.Zero(t, rp.inner.Writes())
	require.Empty(t, rp.rec.Statements())
}
