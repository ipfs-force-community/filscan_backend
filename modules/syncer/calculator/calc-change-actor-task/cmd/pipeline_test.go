package actoractionscmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

// 本文件用**真实的 CalcChangeActorTask** 跑完整条 Dry 清单管线（直接对应生产要补的那 1333 个高度），
// 是「actor-actions 子命令真能补 chain.actor_actions、只写派生表、--no-write 下零写入、
// 单个坏高度不阻断整批」的主证据。
//
// 高度刻意取非 120 整数倍（避免走到与累计快照有关的分支）；均低于假聚合器链头。

const (
	headEpoch = 6409045 // 假聚合器链头（head-1 才是可回放的最高高度）
	replayA   = 6357058
	replayB   = 6357060
)

// actorBalances 造某高度两个变动 actor 的余额行（chain.actor_balances）
func actorBalances(epoch int64) []*po.ActorBalance {
	return []*po.ActorBalance{
		{Epoch: epoch, ActorId: "t0100", Balance: decimal.NewFromInt(1000)},
		{Epoch: epoch, ActorId: "t0101", Balance: decimal.NewFromInt(2000)},
	}
}

// actorStates 适配器按地址返回的 actor 状态（PrepareActor 读 ActorID / ActorAddr / Balance / Code）
func actorStates() map[chain.SmartAddress]*londobell.ActorState {
	return map[chain.SmartAddress]*londobell.ActorState{
		"t0100": {ActorID: "t0100", ActorAddr: "t0100", ActorType: "account", Balance: decimal.NewFromInt(1000)},
		"t0101": {ActorID: "t0101", ActorAddr: "t0101", ActorType: "account", Balance: decimal.NewFromInt(2000)},
	}
}

// actorReplay 一次清单回放的装配结果
type actorReplay struct {
	rec     *offlinereplay.Recorder
	db      *gorm.DB
	plan    offlinereplay.Plan
	target  offlinereplay.Target
	writes  func() []offlinereplay.WriteStat
	agg     *offlinereplay.FakeAgg
	adapter *offlinereplay.FakeAdapter
	inner   *fakeInnerRepo
}

// buildActorReplay 用真实计算器 + 假仓储/假聚合器/假适配器 + 假连接（不下发任何 SQL）装配清单回放。
func buildActorReplay(t *testing.T, noWrite bool, heights ...int64) *actorReplay {
	t.Helper()

	inner := &fakeInnerRepo{balances: actorBalances(replayA)}
	restore := swapRepo(inner)
	t.Cleanup(restore)

	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)
	agg := offlinereplay.NewFakeAgg(headEpoch, nil)
	adapter := &offlinereplay.FakeAdapter{StateByActor: actorStates()}

	target, writes := buildTarget(db, noWrite)

	text := ""
	for _, h := range heights {
		text += fmt.Sprintf("%d\n", h)
	}
	path := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
	plan, err := offlinereplay.ResolvePlan(0, 0, path)
	require.NoError(t, err)

	return &actorReplay{rec: rec, db: db, plan: plan, target: target, writes: writes, agg: agg, adapter: adapter, inner: inner}
}

// execute 跑一次回放并返回打印出来的报告
func (r *actorReplay) execute(t *testing.T, opt offlinereplay.RunOptions) string {
	t.Helper()
	if opt.ErrorWait == 0 {
		opt.ErrorWait = time.Millisecond
	}
	return captureStdout(t, func() {
		require.NoError(t, offlinereplay.Execute(r.plan, opt, r.target, r.db, r.agg, r.adapter, r.writes))
	})
}

// captureStdout 抓取 fn 期间写入 os.Stdout 的内容（Execute 用 fmt.Println 打印报告）
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

// --no-write：真实计算器跑满清单，chain.actor_actions / chain.actors 的写入只被计数、
// 一次都没转发到真仓储，且一条 SQL 都没下发（同步指针 / 台账 / 任务高度都不写）。
func TestActorActionsListModeNoWriteCountsWithoutAnyWrite(t *testing.T) {
	rp := buildActorReplay(t, true, replayA, replayB)

	report := rp.execute(t, offlinereplay.RunOptions{NoWrite: true})

	require.Zero(t, rp.inner.Writes(), "真写通道一次都不该被碰")
	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
	require.Empty(t, rp.agg.EpochsRequested(),
		"actor 链路与生产同步器一致：不注入 traces，不该有 Traces 调用")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range rp.writes() {
		stats[s.Table] = s
	}
	// 每高度 2 个变动 actor 各一行 actor_actions / 一行 actors（先按 id 删再插 ⇒ 删除 1 次）
	require.Equal(t, offlinereplay.WriteStat{Table: TableActorActions, Calls: 2, Rows: 4, Deletes: 0}, stats[TableActorActions])
	require.Equal(t, offlinereplay.WriteStat{Table: TableActors, Calls: 2, Rows: 4, Deletes: 2}, stats[TableActors])
	require.Equal(t, offlinereplay.WriteStat{Table: TableActorBalances}, stats[TableActorBalances],
		"本命令不跑 change-actor-task ⇒ 不碰 chain.actor_balances")

	require.Contains(t, report, "同步器/任务     : actor / calc-change-actor-task")
	require.Contains(t, report, "2 个高度（升序去重后）")
	require.Contains(t, report, "逐高度结果     : 已跑 2/2 个高度；放弃 0 个；未跑 0 个")
	require.Contains(t, report, "chain.actor_actions 写 2 次/4 行；删除 0 次")
	require.Contains(t, report, "chain.actors 写 2 次/4 行；删除 2 次")
	require.Contains(t, report, "预计写入行数   : 真写模式下即为 8 行")
}

// 不加 --no-write（真写通道 = 假仓储）时：派生表写入真下发，但 Dry 依然保证
// 不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都没有）。
func TestActorActionsDryModeWritesDerivedTablesOnly(t *testing.T) {
	rp := buildActorReplay(t, false, replayA, replayB)
	rp.execute(t, offlinereplay.RunOptions{})

	require.Equal(t, 2, len(rp.inner.addActions), "每个高度一次 AddActorActions")
	require.Equal(t, 2, len(rp.inner.addActors))
	require.Equal(t, 2, len(rp.inner.deleteActors))
	require.Zero(t, len(rp.inner.addBalances), "不写 chain.actor_balances")

	for i, actions := range rp.inner.addActions {
		wantEpoch := []int64{replayA, replayB}[i]
		require.Len(t, actions, 2)
		for _, a := range actions {
			require.Equal(t, wantEpoch, a.Epoch, "写入的高度必须就是清单里的高度")
			require.Equal(t, po.ActorActionNew, a.Action, "chain.actors 里还没有这些 id ⇒ 新增(1)")
		}
	}

	require.Empty(t, rp.rec.Statements(),
		"Dry 模式不写 chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs（一条 SQL 都不该有）")
}

// chain.actors 已有该 id 时，action 记为 更新(2)（该判据反映的是「当前库」状态，命令说明里已写明）
func TestActorActionsMarksUpdateWhenActorExists(t *testing.T) {
	rp := buildActorReplay(t, false, replayA)
	rp.inner.existingActors = []*po.ActorPo{{Id: "t0100"}, {Id: "t0101"}}
	rp.execute(t, offlinereplay.RunOptions{})

	require.Len(t, rp.inner.addActions, 1)
	for _, a := range rp.inner.addActions[0] {
		require.Equal(t, po.ActorActionUpdate, a.Action)
	}
}

// 计算器在一个高度上持续报错（如该高度 actor 余额查不到）：
// 该高度按上限重试后被放弃，其余高度照跑，Execute 不把整批判失败。
func TestActorActionsCalculatorErrorDoesNotFailTheBatch(t *testing.T) {
	const bad = replayA + 1

	rp := buildActorReplay(t, false, replayA, bad, replayB)
	rp.inner.balancesErr = errFakeBalances
	rp.inner.balancesErrEpoch = bad

	report := rp.execute(t, offlinereplay.RunOptions{EpochFailLimit: 2})

	// 坏高度重试 2 次（达上限）后放弃；另外两个高度各查一次余额
	require.Equal(t, 4, rp.inner.getBalancesCall, "坏高度 2 次 + 两个健康高度各 1 次")
	require.Equal(t, 2, len(rp.inner.addActions), "只有两个健康高度补上了 chain.actor_actions")
	var patched []int64
	for _, actions := range rp.inner.addActions {
		patched = append(patched, actions[0].Epoch)
	}
	require.Equal(t, []int64{replayA, replayB}, patched, "补上的正是那两个健康高度（坏高度不在其中）")

	require.Empty(t, rp.rec.Statements(), "报错也不得写指针/台账/任务高度")
	require.Contains(t, report, "逐高度结果     : 已跑 3/3 个高度；放弃 1 个；未跑 0 个")
	require.Contains(t, report, fmt.Sprintf("- 高度 %d 失败 2 次", bad))
	require.Contains(t, report, errFakeBalances.Error())
	require.Contains(t, report, "续跑提示")
}

// 高于链头的高度在开跑前被拦下（清单模式不会去等未来高度，避免整批卡死）
func TestActorActionsRejectsHeightsAboveHead(t *testing.T) {
	rp := buildActorReplay(t, false, replayA, headEpoch+1)

	err := offlinereplay.Execute(rp.plan, offlinereplay.RunOptions{NoWrite: true, ErrorWait: time.Millisecond},
		rp.target, rp.db, rp.agg, rp.adapter, rp.writes)
	require.Error(t, err)
	require.Contains(t, err.Error(), fmt.Sprintf("高于聚合器链头 %d", headEpoch-1))
	require.Zero(t, rp.inner.Writes())
	require.Empty(t, rp.rec.Statements())
}
