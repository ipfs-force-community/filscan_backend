package actoractionscmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

// fakeInnerRepo 假仓储：记录所有写方法是否被调用（用于证明 --no-write 下「一次都没转发到真仓储」）。
// 只实现本包用到的读方法，其余由嵌入接口兜底（未实现的方法一旦被调用即 panic —— 用例在没走到的
// 分支上不允许假绿）。
type fakeInnerRepo struct {
	repository.ChangeActorTask

	addActors        [][]*po.ActorPo
	addActions       [][]*po.ActorAction
	addBalances      [][]*po.ActorBalance
	deleteActors     [][]string
	deleteActions    []chain.Epoch
	deleteBalances   []chain.Epoch
	getBalancesCall  int
	balances         []*po.ActorBalance
	balancesErr      error
	balancesErrEpoch int64 // 只让该高度查余额报错（0 = 每个高度都报错）
	existingActors   []*po.ActorPo
}

func (f *fakeInnerRepo) GetActorBalances(_ context.Context, epoch chain.Epoch) ([]*po.ActorBalance, error) {
	f.getBalancesCall++
	if f.balancesErr != nil && (f.balancesErrEpoch == 0 || f.balancesErrEpoch == epoch.Int64()) {
		return nil, f.balancesErr
	}
	return f.balances, nil
}

func (f *fakeInnerRepo) GetActorsByIds(_ context.Context, _ []string) ([]*po.ActorPo, error) {
	return f.existingActors, nil
}

func (f *fakeInnerRepo) AddActors(_ context.Context, actors []*po.ActorPo) error {
	f.addActors = append(f.addActors, actors)
	return nil
}

func (f *fakeInnerRepo) AddActorActions(_ context.Context, actions []*po.ActorAction) error {
	f.addActions = append(f.addActions, actions)
	return nil
}

func (f *fakeInnerRepo) AddActorBalances(_ context.Context, balances []*po.ActorBalance) error {
	f.addBalances = append(f.addBalances, balances)
	return nil
}

func (f *fakeInnerRepo) DeleteActorsByIds(_ context.Context, ids []string) error {
	f.deleteActors = append(f.deleteActors, ids)
	return nil
}

func (f *fakeInnerRepo) DeleteActorActions(_ context.Context, e chain.Epoch) error {
	f.deleteActions = append(f.deleteActions, e)
	return nil
}

func (f *fakeInnerRepo) DeleteActorBalances(_ context.Context, e chain.Epoch) error {
	f.deleteBalances = append(f.deleteBalances, e)
	return nil
}

// Writes 已转发到真仓储的写入次数（任何一类都算）
func (f *fakeInnerRepo) Writes() int {
	return len(f.addActors) + len(f.addActions) + len(f.addBalances) +
		len(f.deleteActors) + len(f.deleteActions) + len(f.deleteBalances)
}

// ---- 参数校验：区间 / 清单 二选一，互斥 ----

func TestResolvePlanWiring(t *testing.T) {
	listPath := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(listPath, []byte("6280001\n6280003\n"), 0o600))

	t.Run("清单模式", func(t *testing.T) {
		plan, err := resolvePlan(options{epochsFile: listPath})
		require.NoError(t, err)
		require.True(t, plan.IsList())
		require.Equal(t, []int64{6280001, 6280003}, plan.List.Epochs())
	})

	t.Run("区间模式", func(t *testing.T) {
		plan, err := resolvePlan(options{start: 6280001, end: 6412000})
		require.NoError(t, err)
		require.False(t, plan.IsList())
		require.Equal(t, int64(132000), plan.Count())
	})

	t.Run("--epochs-file 与 --start/--end 同时给出 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{start: 6280001, end: 6412000, epochsFile: listPath})
		require.Error(t, err)
		require.Contains(t, err.Error(), "互斥")
	})

	t.Run("两边都不给 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "--epochs-file")
	})

	t.Run("清单文件不存在 ⇒ 报错（不连任何生产依赖）", func(t *testing.T) {
		_, err := resolvePlan(options{epochsFile: filepath.Join(t.TempDir(), "nope.list")})
		require.Error(t, err)
		require.Contains(t, err.Error(), "读取高度清单")
	})
}

// 命令行契约：三种计划形态的 flag 都在；start/end 不再是「必填 flag」（清单模式下不传它们），
// 互斥由 resolvePlan/ResolvePlan 判定。
func TestCommandFlags(t *testing.T) {
	cmd := Command()

	require.NotNil(t, cmd.Flags().Lookup("config"))
	require.NotNil(t, cmd.Flags().Lookup("start"))
	require.NotNil(t, cmd.Flags().Lookup("end"))
	require.NotNil(t, cmd.Flags().Lookup("epochs-file"))
	require.NotNil(t, cmd.Flags().Lookup("no-write"))

	required := cobra.BashCompOneRequiredFlag
	require.NotNil(t, cmd.Flags().Lookup("config").Annotations[required], "config 仍必填")
	require.Nil(t, cmd.Flags().Lookup("start").Annotations[required], "start 不应再是必填（清单模式不传）")
	require.Nil(t, cmd.Flags().Lookup("end").Annotations[required], "end 不应再是必填（清单模式不传）")
}

// ---- 目标装配 ----

func TestBuildTarget(t *testing.T) {
	inner := &fakeInnerRepo{}
	restore := swapRepo(inner)
	defer restore()

	t.Run("真写：只注册计算器，且与生产 actor 同步器一样不注入 traces", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, false)
		require.Equal(t, filscansyncer.ActorSyncer, target.Name)
		require.Empty(t, target.Groups, "只跑计算器：不重跑 change-actor-task（chain.actor_balances 已就位）")
		require.Len(t, target.Calculators, 1)
		require.Equal(t, "calc-change-actor-task", target.Calculators[0].Name())
		require.True(t, target.SkipTraces, "生产 actor 同步器没有 WithContextBuilder")
		require.Nil(t, writes, "真写模式没有写统计来源（写入直接下发）")
	})

	t.Run("--no-write：仓储被「只统计不落库」包装替换", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, true)
		require.Len(t, target.Calculators, 1)
		require.NotNil(t, writes)
	})
}

// ---- 「只统计不落库」包装 ----

func TestNoWriteChangeActorRepoBlocksEveryWrite(t *testing.T) {
	inner := &fakeInnerRepo{}
	var repo repository.ChangeActorTask = inner
	wrapped := NewNoWriteChangeActorRepo(repo)
	ctx := context.Background()

	// 计算器会走到的四个写方法 + 两个回滚路径写方法：全部被拦下，一次都不转发
	require.NoError(t, wrapped.DeleteActorsByIds(ctx, []string{"t0100", "t0101"}))
	require.NoError(t, wrapped.AddActors(ctx, []*po.ActorPo{{Id: "t0100"}, {Id: "t0101"}}))
	require.NoError(t, wrapped.AddActorActions(ctx, []*po.ActorAction{
		{Epoch: 6280001, ActorId: "t0100", Action: po.ActorActionNew},
		{Epoch: 6280001, ActorId: "t0101", Action: po.ActorActionUpdate},
	}))
	require.NoError(t, wrapped.AddActorBalances(ctx, []*po.ActorBalance{{Epoch: 6280001, ActorId: "t0100"}}))
	require.NoError(t, wrapped.DeleteActorActions(ctx, chain.Epoch(6280001)))
	require.NoError(t, wrapped.DeleteActorBalances(ctx, chain.Epoch(6280001)))

	require.Zero(t, inner.Writes(), "包装层不得把任何写转发到真仓储")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		stats[s.Table] = s
	}
	require.Equal(t, offlinereplay.WriteStat{Table: TableActorActions, Calls: 1, Rows: 2, Deletes: 1}, stats[TableActorActions])
	require.Equal(t, offlinereplay.WriteStat{Table: TableActors, Calls: 1, Rows: 2, Deletes: 1}, stats[TableActors])
	require.Equal(t, offlinereplay.WriteStat{Table: TableActorBalances, Calls: 1, Rows: 1, Deletes: 1}, stats[TableActorBalances])

	// 读方法原样透传（包装只拦写）
	inner.balances = []*po.ActorBalance{{Epoch: 1, ActorId: "t0100"}}
	got, err := wrapped.GetActorBalances(ctx, chain.Epoch(1))
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, 1, inner.getBalancesCall)
}

func TestNewNoWriteChangeActorRepoRejectsNil(t *testing.T) {
	require.Panics(t, func() { NewNoWriteChangeActorRepo(nil) })
}

var errFakeBalances = errors.New("fake: 该高度的 actor 余额查询失败")

// swapRepo 把 buildTarget 用的仓储构造器换成假仓储（生产路径仍是 dal.NewChangeActorTaskDal）
func swapRepo(inner repository.ChangeActorTask) func() {
	old := newChangeActorRepo
	newChangeActorRepo = func(*gorm.DB) repository.ChangeActorTask { return inner }
	return func() { newChangeActorRepo = old }
}
