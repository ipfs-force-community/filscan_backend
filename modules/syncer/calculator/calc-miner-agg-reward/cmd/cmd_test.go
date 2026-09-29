package mineraggrewardcmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

// fakeRewardRepo 假仓储：记录写方法是否被调用（证明 --no-write 下「一次都没转发到真仓储」），
// 同时记录取矿工用的批区间（用于钉住「输入是 chain.miner_rewards 的批区间」）。
type fakeRewardRepo struct {
	repository.RewardTask

	aggRewards [][]*po.MinerAggReward

	deleteMinerRewards []chain.Epoch

	minerRanges []chain.LCRCRange
}

func (f *fakeRewardRepo) GetRewardMiners(_ context.Context, epochs chain.LCRCRange) ([]string, error) {
	f.minerRanges = append(f.minerRanges, epochs)
	return []string{"t0100", "t0101"}, nil
}

func (f *fakeRewardRepo) SumMinersTotalRewards(_ context.Context, miners []string) ([]*po.MinerAggReward, error) {
	out := make([]*po.MinerAggReward, 0, len(miners))
	for _, m := range miners {
		out = append(out, &po.MinerAggReward{Miner: m, AggReward: decimal.NewFromInt(100), AggBlockCount: 2, AggWinCount: 5})
	}
	return out, nil
}

func (f *fakeRewardRepo) SaveMinerAggReward(_ context.Context, aggRewards []*po.MinerAggReward) error {
	f.aggRewards = append(f.aggRewards, aggRewards)
	return nil
}

func (f *fakeRewardRepo) DeleteMinerRewards(_ context.Context, e chain.Epoch) error {
	f.deleteMinerRewards = append(f.deleteMinerRewards, e)
	return nil
}

// Writes 已转发到真仓储的写入次数（任何一类都算）
func (f *fakeRewardRepo) Writes() int {
	return len(f.aggRewards) + len(f.deleteMinerRewards)
}

// ---- 参数校验：区间 / 清单 二选一，互斥；且清单模式对本命令必须被拒绝 ----

func TestResolvePlanWiring(t *testing.T) {
	listPath := filepath.Join(t.TempDir(), "gap.list")
	require.NoError(t, os.WriteFile(listPath, []byte("6357121\n6357123\n"), 0o600))

	t.Run("区间模式（左闭右闭）", func(t *testing.T) {
		plan, err := resolvePlan(options{start: 6357120, end: 6408648})
		require.NoError(t, err)
		require.False(t, plan.IsList())
		require.Equal(t, int64(6408648-6357120+1), plan.Count())
		require.NoError(t, validatePlan(plan))
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

	t.Run("清单模式被显式拒绝（否则必然零写入、报告却显示全部完成）", func(t *testing.T) {
		plan, err := resolvePlan(options{epochsFile: listPath})
		require.NoError(t, err)
		require.True(t, plan.IsList())
		err = validatePlan(plan)
		require.Error(t, err)
		require.Contains(t, err.Error(), "不支持高度清单模式")
		require.Contains(t, err.Error(), "--start/--end")
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
	require.Equal(t, "miner-agg-reward", cmd.Name())
}

// ---- 目标装配 ----

func TestBuildTarget(t *testing.T) {
	inner := &fakeRewardRepo{}
	restore := swapRewardRepo(inner)
	defer restore()

	t.Run("真写：同步器名与生产一致，只注册计算器，且不注入 traces", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, false)
		require.Equal(t, "chain", target.Name)
		require.Empty(t, target.Groups)
		require.Len(t, target.Calculators, 1)
		require.Equal(t, "calc-miner-agg-reward", target.Calculators[0].Name())
		require.True(t, target.SkipTraces)
		require.Nil(t, writes)
	})

	t.Run("--no-write：仓储被「只统计不落库」包装替换", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, true)
		require.Len(t, target.Calculators, 1)
		require.NotNil(t, writes)
	})
}

// ---- 「只统计不落库」包装 ----

func TestNoWriteRewardRepoBlocksEveryWrite(t *testing.T) {
	inner := &fakeRewardRepo{}
	var repo repository.RewardTask = inner
	wrapped := NewNoWriteRewardRepo(repo)
	ctx := context.Background()

	require.NoError(t, wrapped.SaveMinerAggReward(ctx, []*po.MinerAggReward{{Miner: "t0100"}, {Miner: "t0101"}}))
	require.NoError(t, wrapped.DeleteMinerRewards(ctx, chain.Epoch(6357121)))

	require.Zero(t, inner.Writes(), "包装层不得把任何写转发到真仓储")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		stats[s.Table] = s
	}
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerAggRewards, Calls: 1, Rows: 2}, stats[TableMinerAggRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewards, Deletes: 1}, stats[TableMinerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerRewards}, stats[TableOwnerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerWinCounts}, stats[TableMinerWinCounts])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewardStats}, stats[TableMinerRewardStats])

	// 读方法原样透传（包装只拦写）
	got, err := wrapped.GetRewardMiners(ctx, chain.NewLCRCRange(1, 2))
	require.NoError(t, err)
	require.Equal(t, []string{"t0100", "t0101"}, got)
	require.Len(t, inner.minerRanges, 1)
}

func TestNewNoWriteRewardRepoRejectsNil(t *testing.T) {
	require.Panics(t, func() { NewNoWriteRewardRepo(nil) })
}

// swapRewardRepo 把 buildTarget 用的仓储构造器换成假仓储（生产路径仍是 dal.NewRewardTaskDal）
func swapRewardRepo(inner repository.RewardTask) func() {
	old := newRewardRepo
	newRewardRepo = func(*gorm.DB) repository.RewardTask { return inner }
	return func() { newRewardRepo = old }
}
