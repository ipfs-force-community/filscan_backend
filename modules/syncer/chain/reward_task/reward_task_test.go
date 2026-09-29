package reward_task

import (
	"context"
	"fmt"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/miner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// ----- 测试替身：只实现被测路径真正用到的两个方法 -----

// fakeAdapter 只实现 reward-task 用到的 adapter.Miner（取矿工在当前高度的 owner）。
// 其余方法由嵌入式 nil 接口兜底：一旦被调用会立刻 panic 暴露出来。
type fakeAdapter struct {
	londobell.Adapter
	minerOwners map[string]string
}

func (f *fakeAdapter) Miner(_ context.Context, m chain.SmartAddress, _ *chain.Epoch) (*londobell.MinerDetail, error) {
	o, ok := f.minerOwners[m.Address()]
	if !ok {
		return nil, fmt.Errorf("fakeAdapter 未配置矿工 %s 的 owner", m.Address())
	}
	return &londobell.MinerDetail{Miner: m.Address(), Owner: o}, nil
}

// fakeRewardRepo 只实现 prepareOwnerRewards 需要的 GetLastOwnerRewardOrNil，
// 其余方法由嵌入式 nil 接口兜底；calls 记录「取上一条」被调用的次数。
type fakeRewardRepo struct {
	repository.RewardTask
	last  map[string]*owner.Reward
	calls int
}

func (f *fakeRewardRepo) GetLastOwnerRewardOrNil(_ context.Context, _ chain.Epoch, o chain.SmartAddress) (*owner.Reward, error) {
	f.calls++
	if f.last == nil {
		return nil, nil
	}
	return f.last[o.Address()], nil
}

func atto(v int64) chain.AttoFil {
	return chain.AttoFil(decimal.NewFromInt(v))
}

// 回归测试：同一高度、同一 owner 下有 N 个爆块矿工时，
// 「上一条 owner_rewards 的 acc_reward / acc_block_count」只能累加**一次**。
//
// 旧实现把「取上一条并累加 acc」写在按**矿工**遍历的循环体内（reward_task.go:159-168），
// 于是同一 owner 在本高度有 3 个矿工时，上一条 acc 被加 3 次：
//
//	acc_reward      = (10+20+30) + 1000×3 = 3060（应为 1060）
//	acc_block_count = (1+2+3)   + 7×3    =  27（应为 13）
//
// 在连续高度上每次都放大 N 倍 ⇒ 生产上 acc_reward 已到 10^1500 量级、
// acc_block_count 已 int64 溢出为负数。本测试在旧代码上必然失败（红）。
func TestPrepareOwnerRewardsAddsPreviousRowOncePerOwner(t *testing.T) {
	const epoch = chain.Epoch(700000)

	adapter := &fakeAdapter{minerOwners: map[string]string{
		"f0100": "f0200", // 同一 owner 下的 3 个矿工
		"f0101": "f0200",
		"f0102": "f0200",
		"f0103": "f0300", // 另一个 owner，只有 1 个矿工
	}}
	repo := &fakeRewardRepo{last: map[string]*owner.Reward{
		"f0200": {
			Epoch:         epoch - 120,
			Owner:         "f0200",
			AccReward:     atto(1000),
			AccBlockCount: 7,
		},
		"f0300": {
			Epoch:         epoch - 500,
			Owner:         "f0300",
			AccReward:     atto(5000),
			AccBlockCount: 9,
		},
	}}

	ctx := syncer.NewTestContext(adapter, nil, epoch)
	minerRewards := []*miner.Reward{
		{Epoch: epoch, Miner: "f0100", Reward: atto(10), BlockCount: 1},
		{Epoch: epoch, Miner: "f0101", Reward: atto(20), BlockCount: 2},
		{Epoch: epoch, Miner: "f0102", Reward: atto(30), BlockCount: 3},
		{Epoch: epoch, Miner: "f0103", Reward: atto(40), BlockCount: 4},
	}

	got, err := NewMinerRewardTask(repo).prepareOwnerRewards(ctx, minerRewards)
	require.NoError(t, err)
	require.Len(t, got, 2, "每个 owner 一行")

	byOwner := map[string]*owner.Reward{}
	for _, v := range got {
		byOwner[v.Owner.Address()] = v
	}

	// 同一高度 3 个矿工同属 f0200：上一条只加一次
	a := byOwner["f0200"]
	require.NotNil(t, a)
	require.Equal(t, 0, a.Reward.Decimal().Cmp(decimal.NewFromInt(60)), "本高度奖励之和 = 10+20+30")
	require.Equal(t, int64(6), a.BlockCount, "本高度爆块数之和 = 1+2+3")
	require.Equal(t, 0, a.AccReward.Decimal().Cmp(decimal.NewFromInt(1060)),
		"acc_reward 必须 = 本高度 60 + 上一条 1000（不得 ×3）")
	require.Equal(t, int64(13), a.AccBlockCount, "acc_block_count 必须 = 6 + 7（不得 ×3）")
	require.Equal(t, int64(epoch-120), a.PrevEpochRef.Int64(), "prev_epoch_ref 指向上一条")
	require.Equal(t, epoch.Int64(), a.Epoch.Int64())
	require.Len(t, a.Miners, 3, "同一 owner 下的矿工都记在 miners 里")

	// 另一个 owner 只有 1 个矿工：单矿工路径的结果不受影响
	b := byOwner["f0300"]
	require.NotNil(t, b)
	require.Equal(t, 0, b.AccReward.Decimal().Cmp(decimal.NewFromInt(5040)))
	require.Equal(t, int64(13), b.AccBlockCount)
	require.Equal(t, int64(epoch-500), b.PrevEpochRef.Int64())

	require.Equal(t, 2, repo.calls, "每个 owner 只查一次上一条 owner_rewards（旧实现是 4 次）")
}

// 查不到上一条记录时：acc_* 就是本高度的值，prev_epoch_ref 保持当前高度（语义不变）
func TestPrepareOwnerRewardsWithoutPreviousRow(t *testing.T) {
	const epoch = chain.Epoch(700000)

	adapter := &fakeAdapter{minerOwners: map[string]string{"f0100": "f0200", "f0101": "f0200"}}
	repo := &fakeRewardRepo{} // last 为空 ⇒ 永远返回 nil

	ctx := syncer.NewTestContext(adapter, nil, epoch)
	minerRewards := []*miner.Reward{
		{Epoch: epoch, Miner: "f0100", Reward: atto(10), BlockCount: 1},
		{Epoch: epoch, Miner: "f0101", Reward: atto(20), BlockCount: 2},
	}

	got, err := NewMinerRewardTask(repo).prepareOwnerRewards(ctx, minerRewards)
	require.NoError(t, err)
	require.Len(t, got, 1)

	require.Equal(t, 0, got[0].AccReward.Decimal().Cmp(decimal.NewFromInt(30)))
	require.Equal(t, int64(3), got[0].AccBlockCount)
	require.Equal(t, epoch.Int64(), got[0].PrevEpochRef.Int64(), "没有上一条时 prev_epoch_ref = 当前高度")
	require.Equal(t, epoch.Int64(), got[0].SyncMinerRef.Int64())
	require.Equal(t, 1, repo.calls)
}

// 连续两个高度（同一 owner 下多矿工）：第一步的累计值即第二步的基准，
// 不会出现「每步放大 N 倍」的复利效应。
func TestPrepareOwnerRewardsCompoundsLinearlyAcrossEpochs(t *testing.T) {
	adapter := &fakeAdapter{minerOwners: map[string]string{"f0100": "f0200", "f0101": "f0200"}}
	repo := &fakeRewardRepo{last: map[string]*owner.Reward{
		"f0200": {Epoch: 699999, Owner: "f0200", AccReward: atto(1000), AccBlockCount: 10},
	}}

	task := NewMinerRewardTask(repo)
	rewards := []*miner.Reward{
		{Epoch: 700000, Miner: "f0100", Reward: atto(10), BlockCount: 1},
		{Epoch: 700000, Miner: "f0101", Reward: atto(20), BlockCount: 2},
	}

	ctx := syncer.NewTestContext(adapter, nil, 700000)
	first, err := task.prepareOwnerRewards(ctx, rewards)
	require.NoError(t, err)
	require.Equal(t, 0, first[0].AccReward.Decimal().Cmp(decimal.NewFromInt(1030)), "1000 + 30")
	require.Equal(t, int64(13), first[0].AccBlockCount, "10 + 3")

	// 用第一步的结果作为高度的「上一条」，再跑一步（同一高度两个矿工，共 2 个矿工 = 2 倍系数）
	repo.last["f0200"] = first[0]
	ctx2 := syncer.NewTestContext(adapter, nil, 700001)
	second, err := task.prepareOwnerRewards(ctx2, []*miner.Reward{
		{Epoch: 700001, Miner: "f0100", Reward: atto(10), BlockCount: 1},
		{Epoch: 700001, Miner: "f0101", Reward: atto(20), BlockCount: 2},
	})
	require.NoError(t, err)
	require.Equal(t, 0, second[0].AccReward.Decimal().Cmp(decimal.NewFromInt(1060)), "1030 + 30（线性，不是 ×2）")
	require.Equal(t, int64(16), second[0].AccBlockCount, "13 + 3")
}

// ----- wincount 落库：gas_reward 必须与 win_count 同批落盘 -----

// 聚合器 wincount 的 TotalGasReward 与 TotalWinCount 来自同一次响应，
// 落库时一个都不能丢：丢 gas_reward 会让 PG 路径的 TxFeeReward / MinedReward 恒为 0
// （acl_block_chain.GetBlockDetails 的消费点）。
func TestToMinerWinCountKeepsGasReward(t *testing.T) {
	row := toMinerWinCount(chain.Epoch(6330000), &londobell.MinerWinCount{
		Id:             "03645007", // 聚合器 _id 是不带前缀的 0… 形态
		TotalWinCount:  2,
		TotalGasReward: decimal.RequireFromString("89145023322864"),
	})

	require.Equal(t, int64(6330000), row.Epoch)
	require.Equal(t, "f03645007", row.Miner, "落库统一成带前缀形态")
	require.Equal(t, int64(2), row.WinCount)
	require.Equal(t, "89145023322864", row.GasReward.String(), "attoFIL 原样落库，不做单位换算")
}

// gas_reward 合法为 0 时必须落 0（不是「没值」）：0 与「未回填的 NULL」语义不同，
// 读路径据此判断能不能走 PG。
func TestToMinerWinCountKeepsZeroGasReward(t *testing.T) {
	row := toMinerWinCount(chain.Epoch(6330000), &londobell.MinerWinCount{
		Id:            "02826815",
		TotalWinCount: 1,
	})
	require.True(t, row.GasReward.IsZero())
	require.Equal(t, "0", row.GasReward.String())
}
