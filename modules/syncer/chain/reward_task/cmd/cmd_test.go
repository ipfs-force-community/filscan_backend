package minerrewardscmd

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
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/miner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
)

// fakeRewardRepo 假仓储：记录写方法是否被调用（证明 --no-write 下「一次都没转发到真仓储」）。
// 只实现本包链路会走到的读方法，其余由嵌入接口兜底（未实现的方法一旦被调用即 panic —— 不允许假绿）。
type fakeRewardRepo struct {
	repository.RewardTask

	minerRewards [][]*miner.Reward
	ownerRewards [][]*owner.Reward
	winCounts    [][]*po.MinerWinCount
	rewardStats  [][]*po.MinerRewardStat
	aggRewards   [][]*po.MinerAggReward

	deleteMinerRewards []chain.Epoch
	deleteOwnerRewards []chain.Epoch
	deleteWinCounts    []chain.Epoch
	deleteRewardStats  []chain.Epoch

	lastOwnerReward *owner.Reward
}

func (f *fakeRewardRepo) GetLastOwnerRewardOrNil(_ context.Context, _ chain.Epoch, _ chain.SmartAddress) (*owner.Reward, error) {
	return f.lastOwnerReward, nil
}

func (f *fakeRewardRepo) SaveMinerRewards(_ context.Context, rewards []*miner.Reward) error {
	f.minerRewards = append(f.minerRewards, rewards)
	return nil
}

func (f *fakeRewardRepo) SaveOwnerRewards(_ context.Context, rewards []*owner.Reward) error {
	f.ownerRewards = append(f.ownerRewards, rewards)
	return nil
}

func (f *fakeRewardRepo) SaveWinCounts(_ context.Context, winCounts []*po.MinerWinCount) error {
	f.winCounts = append(f.winCounts, winCounts)
	return nil
}

func (f *fakeRewardRepo) SaveMinerRewardStats(_ context.Context, stats []*po.MinerRewardStat) error {
	f.rewardStats = append(f.rewardStats, stats)
	return nil
}

func (f *fakeRewardRepo) SaveMinerAggReward(_ context.Context, aggRewards []*po.MinerAggReward) error {
	f.aggRewards = append(f.aggRewards, aggRewards)
	return nil
}

func (f *fakeRewardRepo) DeleteMinerRewards(_ context.Context, e chain.Epoch) error {
	f.deleteMinerRewards = append(f.deleteMinerRewards, e)
	return nil
}

func (f *fakeRewardRepo) DeleteOwnerRewards(_ context.Context, e chain.Epoch) error {
	f.deleteOwnerRewards = append(f.deleteOwnerRewards, e)
	return nil
}

func (f *fakeRewardRepo) DeleteWinCounts(_ context.Context, e chain.Epoch) error {
	f.deleteWinCounts = append(f.deleteWinCounts, e)
	return nil
}

func (f *fakeRewardRepo) DeleteMinerRewardStats(_ context.Context, e chain.Epoch) error {
	f.deleteRewardStats = append(f.deleteRewardStats, e)
	return nil
}

// Writes 已转发到真仓储的写入次数（任何一类都算）
func (f *fakeRewardRepo) Writes() int {
	return len(f.minerRewards) + len(f.ownerRewards) + len(f.winCounts) + len(f.rewardStats) + len(f.aggRewards) +
		len(f.deleteMinerRewards) + len(f.deleteOwnerRewards) + len(f.deleteWinCounts) + len(f.deleteRewardStats)
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

// 命令行契约：四种 flag 都在；start/end 不再是「必填 flag」（清单模式下不传它们），互斥由 ResolvePlan 判定。
func TestCommandFlags(t *testing.T) {
	cmd := Command()

	require.NotNil(t, cmd.Flags().Lookup("config"))
	require.NotNil(t, cmd.Flags().Lookup("start"))
	require.NotNil(t, cmd.Flags().Lookup("end"))
	require.NotNil(t, cmd.Flags().Lookup("epochs-file"))
	require.NotNil(t, cmd.Flags().Lookup("no-write"))

	required := cobra.BashCompOneRequiredFlag
	require.NotNil(t, cmd.Flags().Lookup("config").Annotations[required], "config 仍必填")
	require.Nil(t, cmd.Flags().Lookup("start").Annotations[required], "start 不应是必填（清单模式不传）")
	require.Nil(t, cmd.Flags().Lookup("end").Annotations[required], "end 不应是必填（清单模式不传）")
	require.Equal(t, "miner-rewards", cmd.Name())
}

// ---- 目标装配 ----

func TestBuildTarget(t *testing.T) {
	inner := &fakeRewardRepo{}
	restore := swapRepo(inner)
	defer restore()

	t.Run("真写：同步器名与生产一致，只注册 reward-task，不注入 traces", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, false)
		require.Equal(t, filscansyncer.ChainSyncer, target.Name)
		require.Len(t, target.Groups, 1)
		require.Len(t, target.Groups[0], 1)
		require.Equal(t, "reward-task", target.Groups[0][0].Name())
		require.Empty(t, target.Calculators, "本命令只跑 sync task，不跑 chain 同步器的计算器")
		require.True(t, target.SkipTraces, "reward-task 不读 traces")
		require.Nil(t, writes, "真写模式没有写统计来源（写入直接下发）")
	})

	t.Run("--no-write：仓储被「只统计不落库」包装替换", func(t *testing.T) {
		target, writes := buildTarget(&gorm.DB{}, true)
		require.Len(t, target.Groups[0], 1)
		require.NotNil(t, writes)
	})
}

// ---- 「只统计不落库」包装 ----

func TestNoWriteRewardRepoBlocksEveryWrite(t *testing.T) {
	inner := &fakeRewardRepo{}
	var repo repository.RewardTask = inner
	wrapped := NewNoWriteRewardRepo(repo)
	ctx := context.Background()

	require.NoError(t, wrapped.SaveMinerRewards(ctx, []*miner.Reward{{Epoch: 6357121}, {Epoch: 6357121}}))
	require.NoError(t, wrapped.SaveOwnerRewards(ctx, []*owner.Reward{{Epoch: 6357121}}))
	require.NoError(t, wrapped.SaveWinCounts(ctx, []*po.MinerWinCount{{Epoch: 6357121}, {Epoch: 6357121}}))
	require.NoError(t, wrapped.SaveMinerRewardStats(ctx, []*po.MinerRewardStat{{Epoch: 6357121}}))
	require.NoError(t, wrapped.SaveMinerAggReward(ctx, []*po.MinerAggReward{{Miner: "t0100"}}))
	require.NoError(t, wrapped.DeleteMinerRewards(ctx, chain.Epoch(6357121)))
	require.NoError(t, wrapped.DeleteOwnerRewards(ctx, chain.Epoch(6357121)))
	require.NoError(t, wrapped.DeleteWinCounts(ctx, chain.Epoch(6357121)))
	require.NoError(t, wrapped.DeleteMinerRewardStats(ctx, chain.Epoch(6357121)))

	require.Zero(t, inner.Writes(), "包装层不得把任何写转发到真仓储")

	stats := map[string]offlinereplay.WriteStat{}
	for _, s := range wrapped.WriteStats() {
		stats[s.Table] = s
	}
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewards, Calls: 1, Rows: 2, Deletes: 1}, stats[TableMinerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableOwnerRewards, Calls: 1, Rows: 1, Deletes: 1}, stats[TableOwnerRewards])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerWinCounts, Calls: 1, Rows: 2, Deletes: 1}, stats[TableMinerWinCounts])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerRewardStats, Calls: 1, Rows: 1, Deletes: 1}, stats[TableMinerRewardStats])
	require.Equal(t, offlinereplay.WriteStat{Table: TableMinerAggRewards, Calls: 1, Rows: 1, Deletes: 0}, stats[TableMinerAggRewards])

	// 读方法原样透传（包装只拦写）
	inner.lastOwnerReward = &owner.Reward{Epoch: 6357000, AccReward: chain.AttoFil(decimal.NewFromInt(7))}
	got, err := wrapped.GetLastOwnerRewardOrNil(ctx, chain.Epoch(6357121), "t0100")
	require.NoError(t, err)
	require.Equal(t, int64(6357000), got.Epoch.Int64())
}

func TestNewNoWriteRewardRepoRejectsNil(t *testing.T) {
	require.Panics(t, func() { NewNoWriteRewardRepo(nil) })
}

// swapRepo 把 buildTarget 用的仓储构造器换成假仓储（生产路径仍是 dal.NewRewardTaskDal）
func swapRepo(inner repository.RewardTask) func() {
	old := newRewardRepo
	newRewardRepo = func(*gorm.DB) repository.RewardTask { return inner }
	return func() { newRewardRepo = old }
}
