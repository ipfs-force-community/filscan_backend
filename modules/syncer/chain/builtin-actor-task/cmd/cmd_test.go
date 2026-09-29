package baselineactorscmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

// fakeBaselineRepo 假仓储：记录写方法是否被调用（证明 --no-write 下「一次都没转发到真仓储」）。
type fakeBaselineRepo struct {
	repository.BaselineTaskRepo

	saves   [][]*po.BuiltinActorStatePo
	deletes []chain.Epoch
}

func (f *fakeBaselineRepo) SaveBuiltActorStates(_ context.Context, item ...*po.BuiltinActorStatePo) error {
	f.saves = append(f.saves, item)
	return nil
}

func (f *fakeBaselineRepo) DeleteBuiltActorStates(_ context.Context, e chain.Epoch) error {
	f.deletes = append(f.deletes, e)
	return nil
}

// Writes 已转发到真仓储的写入次数（任何一类都算）
func (f *fakeBaselineRepo) Writes() int { return len(f.saves) + len(f.deletes) }

// ---- 参数校验：区间 / 清单 二选一，互斥 ----

func TestResolvePlanWiring(t *testing.T) {
	listPath := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(listPath, []byte("6357121\n6357123\n"), 0o600))

	t.Run("清单模式", func(t *testing.T) {
		plan, err := resolvePlan(options{epochsFile: listPath})
		require.NoError(t, err)
		require.True(t, plan.IsList())
		require.Equal(t, []int64{6357121, 6357123}, plan.List.Epochs())
	})

	t.Run("区间模式（左闭右闭）", func(t *testing.T) {
		plan, err := resolvePlan(options{start: 6357120, end: 6408648})
		require.NoError(t, err)
		require.False(t, plan.IsList())
		require.Equal(t, int64(6408648-6357120+1), plan.Count())
	})

	t.Run("--epochs-file 与 --start/--end 同时给出 ⇒ 报错", func(t *testing.T) {
		_, err := resolvePlan(options{start: 6357120, end: 6408648, epochsFile: listPath})
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

func TestCommandFlags(t *testing.T) {
	cmd := Command()

	require.NotNil(t, cmd.Flags().Lookup("config"))
	require.NotNil(t, cmd.Flags().Lookup("start"))
	require.NotNil(t, cmd.Flags().Lookup("end"))
	require.NotNil(t, cmd.Flags().Lookup("epochs-file"))
	require.NotNil(t, cmd.Flags().Lookup("no-write"))

	required := cobra.BashCompOneRequiredFlag
	require.NotNil(t, cmd.Flags().Lookup("config").Annotations[required], "config 仍必填")
	require.Nil(t, cmd.Flags().Lookup("start").Annotations[required], "start 不应是必填")
	require.Nil(t, cmd.Flags().Lookup("end").Annotations[required], "end 不应是必填")
	require.Equal(t, "baseline-actors", cmd.Name())
}

// ---- 目标装配 ----

func TestBuildTarget(t *testing.T) {
	inner := &fakeBaselineRepo{}
	restore := restoreBaselineRepo(inner)
	defer restore()

	t.Run("真写：同步器名与生产一致，只注册 baseline-task，且不注入 traces", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, false)
		require.Equal(t, "chain", target.Name)
		require.Len(t, target.Groups, 1)
		require.Len(t, target.Groups[0], 1)
		require.Equal(t, "baseline-task", target.Groups[0][0].Name())
		require.Empty(t, target.Calculators, "本命令只跑 sync task，不跑 chain 同步器的计算器")
		require.True(t, target.SkipTraces, "baseline-task 不读 traces（也只用适配器，无数据库输入）")
		require.Nil(t, writes, "真写模式没有写统计来源（写入直接下发）")
	})

	t.Run("--no-write：仓储被「只统计不落库」包装替换", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, true)
		require.Len(t, target.Groups[0], 1)
		require.NotNil(t, writes)
	})
}

// ---- 「只统计不落库」包装 ----

func TestNoWriteBaselineRepoBlocksEveryWrite(t *testing.T) {
	inner := &fakeBaselineRepo{}
	var repo repository.BaselineTaskRepo = inner
	wrapped := NewNoWriteBaselineRepo(repo)
	ctx := context.Background()

	require.NoError(t, wrapped.SaveBuiltActorStates(ctx,
		&po.BuiltinActorStatePo{Epoch: 6357121, Actor: "x"},
		&po.BuiltinActorStatePo{Epoch: 6357121, Actor: "y"}))
	require.NoError(t, wrapped.DeleteBuiltActorStates(ctx, chain.Epoch(6357121)))

	require.Zero(t, inner.Writes(), "包装层不得把任何写转发到真仓储")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		stats[s.Table] = s
	}
	require.Equal(t, offlinereplay.WriteStat{Table: TableBuiltinActorStates, Calls: 1, Rows: 2, Deletes: 1}, stats[TableBuiltinActorStates])
}

func TestNewNoWriteBaselineRepoRejectsNil(t *testing.T) {
	require.Panics(t, func() { NewNoWriteBaselineRepo(nil) })
}

// restoreBaselineRepo 把 buildTarget 用的仓储构造器换成假仓储（生产路径仍是 dal.NewBaseLineTaskDal）
func restoreBaselineRepo(inner repository.BaselineTaskRepo) func() {
	old := newBaselineRepo
	newBaselineRepo = func(*gorm.DB) repository.BaselineTaskRepo { return inner }
	return func() { newBaselineRepo = old }
}
