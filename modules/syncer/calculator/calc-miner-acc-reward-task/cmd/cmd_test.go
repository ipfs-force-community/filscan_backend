package mineraccrewardcmd

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
// 同时记录两个读方法被调用的区间/高度（用于钉住「本计算器的输入是 chain.miner_rewards +
// chain.builtin_actor_states」）。
type fakeRewardRepo struct {
	repository.RewardTask

	rewardStats [][]*po.MinerRewardStat

	deleteRewardStats []chain.Epoch

	sumRanges  []chain.LCRCRange
	powerEpoch []chain.Epoch
	// emptyInputs 模拟「chain.miner_rewards 与 chain.builtin_actor_states 都为空」：
	// SumRewards 返回 0（真实 SQL 的 greatest(sum(reward),0) 在无行时就是 0），全网算力返回 0。
	emptyInputs bool
}

func (f *fakeRewardRepo) SumRewards(_ context.Context, epochs chain.LCRCRange) (decimal.Decimal, error) {
	f.sumRanges = append(f.sumRanges, epochs)
	if f.emptyInputs {
		return decimal.Zero, nil
	}
	return decimal.NewFromInt(1000), nil
}

func (f *fakeRewardRepo) GetNetQualityAdjPower(_ context.Context, epoch chain.Epoch) (decimal.Decimal, error) {
	f.powerEpoch = append(f.powerEpoch, epoch)
	if f.emptyInputs {
		return decimal.Zero, nil
	}
	return chain.PerT.Mul(decimal.NewFromInt(10)), nil
}

func (f *fakeRewardRepo) SaveMinerRewardStats(_ context.Context, stats []*po.MinerRewardStat) error {
	f.rewardStats = append(f.rewardStats, stats)
	return nil
}

func (f *fakeRewardRepo) DeleteMinerRewardStats(_ context.Context, e chain.Epoch) error {
	f.deleteRewardStats = append(f.deleteRewardStats, e)
	return nil
}

// Writes 已转发到真仓储的写入次数（任何一类都算）
func (f *fakeRewardRepo) Writes() int {
	return len(f.rewardStats) + len(f.deleteRewardStats)
}

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
	require.Equal(t, "miner-acc-reward", cmd.Name())
}

// ---- 目标装配 ----

func TestBuildTarget(t *testing.T) {
	inner := &fakeRewardRepo{}
	restore := swapRewardRepo(inner)
	defer restore()

	t.Run("真写：同步器名与生产一致，只注册计算器，且不注入 traces", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, false)
		require.Equal(t, "chain", target.Name)
		require.Empty(t, target.Groups, "只跑计算器：不重跑 chain 同步器的同步任务")
		require.Len(t, target.Calculators, 1)
		require.Equal(t, "calc-miner-acc-reward-task", target.Calculators[0].Name())
		require.True(t, target.SkipTraces, "该计算器既不读 traces 也不用聚合器/适配器")
		require.Nil(t, writes, "真写模式没有写统计来源（写入直接下发）")
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

	require.NoError(t, wrapped.SaveMinerRewardStats(ctx, []*po.MinerRewardStat{
		{Epoch: 6357121, Interval: "24h"}, {Epoch: 6357121, Interval: "7d"},
	}))
	require.NoError(t, wrapped.SaveMinerRewards(ctx, nil))
	require.NoError(t, wrapped.DeleteMinerRewardStats(ctx, chain.Epoch(6357121)))

	require.Zero(t, inner.Writes(), "包装层不得把任何写转发到真仓储")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		stats[s.Table] = s
	}
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewardStats, Calls: 1, Rows: 2, Deletes: 1}, stats[TableMinerRewardStats])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewards, Calls: 1, Rows: 0}, stats[TableMinerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerRewards}, stats[TableOwnerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerWinCounts}, stats[TableMinerWinCounts])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerAggRewards}, stats[TableMinerAggRewards])

	// 读方法原样透传（包装只拦写）
	got, err := wrapped.SumRewards(ctx, chain.NewLCRCRange(1, 2))
	require.NoError(t, err)
	require.Equal(t, 0, got.Cmp(decimal.NewFromInt(1000)))
	require.Len(t, inner.sumRanges, 1)
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
