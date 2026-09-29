package probiz

import (
	"testing"

	"github.com/golang-module/carbon/v2"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	probo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func attoFil(v int64) chain.AttoFil {
	return chain.AttoFil(decimal.NewFromInt(v))
}

func newDayStat(t *testing.T, day string, miner chain.SmartAddress, blocks, winCounts int64, reward, totalReward int64) *merger.DayRewardStat {
	t.Helper()
	d, err := chain.NewDate(carbon.Shanghai, day)
	require.NoError(t, err)
	return &merger.DayRewardStat{
		Day: d,
		Stats: map[chain.SmartAddress]*merger.RewardStat{
			miner: {
				Miner:        miner,
				Blocks:       blocks,
				WinCounts:    winCounts,
				Rewards:      attoFil(reward),
				TotalRewards: attoFil(totalReward),
			},
		},
	}
}

// 「All」行累加字段写错的缺陷回归（缺陷一）：
//
//	旧实现：all.TotalReward = day.TotalRewards.Decimal().Add(all.BlockReward)
//	          —— 累加的是「出块奖励」，于是 All 行的 total_reward 退化成出块奖励的运行和
//	             （生产上 total_reward 恒 0 时，它恰好等于 All 行的 block_reward）。
//	正确口径：四列各自求和。
func TestBuildRewardDetailsAccumulatesEveryColumnIndependently(t *testing.T) {
	miner := chain.SmartAddress("f01000001")
	minersReward := []*merger.DayRewardStat{
		newDayStat(t, "2026-09-27", miner, 2, 3, 15, 100),
		newDayStat(t, "2026-09-28", miner, 1, 1, 5, 200),
	}
	minerInfo := map[chain.SmartAddress]probo.UserMiner{
		miner: {GroupID: 7, GroupName: "pool-a", IsDefault: false, MinerID: miner, MinerTag: "tag-a"},
	}

	list, all := buildRewardDetails([]chain.SmartAddress{miner}, minersReward, minerInfo)

	// 明细行：逐日值原样返回（修复不应改动逐日口径）
	require.Len(t, list, 2)
	require.Equal(t, int64(2), list[0].BlockCount)
	require.Equal(t, int64(3), list[0].WinCount)
	require.True(t, decimal.NewFromInt(15).Equal(list[0].BlockReward))
	require.True(t, decimal.NewFromInt(100).Equal(list[0].TotalReward))
	require.True(t, decimal.NewFromInt(200).Equal(list[1].TotalReward))
	require.Equal(t, "tag-a", list[0].Tag)
	require.Equal(t, "pool-a", list[0].GroupName)
	require.Equal(t, miner, list[0].MinerId)

	// All 行：逐列求和
	require.Equal(t, "All", all.Date)
	require.Equal(t, miner, all.MinerId)
	require.Equal(t, int64(3), all.BlockCount)
	require.Equal(t, int64(4), all.WinCount)
	require.True(t, decimal.NewFromInt(20).Equal(all.BlockReward), "All 行 block_reward = Σ 当日出块奖励")
	require.True(t, decimal.NewFromInt(300).Equal(all.TotalReward), "All 行 total_reward = Σ 当日 total_reward")

	// 旧公式的结果（200 + (15+5) = 220）与正确值不同 —— 旧行为确实是错的
	legacyBlocks, legacyTotal := decimal.Zero, decimal.Zero
	for _, chance := range minersReward {
		stat := chance.Stats[miner]
		legacyBlocks = stat.Rewards.Decimal().Add(legacyBlocks)     // 旧代码：all.BlockReward
		legacyTotal = stat.TotalRewards.Decimal().Add(legacyBlocks) // 旧代码：累加到 all.BlockReward 上
	}
	require.True(t, decimal.NewFromInt(220).Equal(legacyTotal), "旧实现的结果应为 220")
	require.False(t, legacyTotal.Equal(all.TotalReward), "新旧结果必须不同：旧 220 / 新 300")
}

// 多矿工 × 多日展开（RewardDetail 的原始循环形态）：All 行仍是逐列求和，明细行数为 矿工数 × 天数。
func TestBuildRewardDetailsMultipleMiners(t *testing.T) {
	a := chain.SmartAddress("f01000001")
	b := chain.SmartAddress("f01000002")
	minerInfo := map[chain.SmartAddress]probo.UserMiner{
		a: {MinerID: a, MinerTag: "tag-a"},
		b: {MinerID: b, MinerTag: "tag-b"},
	}
	minersReward := []*merger.DayRewardStat{
		{
			Day: func() chain.Date {
				d, err := chain.NewDate(carbon.Shanghai, "2026-09-27")
				require.NoError(t, err)
				return d
			}(),
			Stats: map[chain.SmartAddress]*merger.RewardStat{
				a: {Miner: a, Blocks: 1, WinCounts: 1, Rewards: attoFil(1), TotalRewards: attoFil(10)},
				b: {Miner: b, Blocks: 2, WinCounts: 2, Rewards: attoFil(2), TotalRewards: attoFil(20)},
			},
		},
		{
			Day: func() chain.Date {
				d, err := chain.NewDate(carbon.Shanghai, "2026-09-28")
				require.NoError(t, err)
				return d
			}(),
			Stats: map[chain.SmartAddress]*merger.RewardStat{
				a: {Miner: a, Blocks: 3, WinCounts: 3, Rewards: attoFil(3), TotalRewards: attoFil(30)},
				b: {Miner: b, Blocks: 4, WinCounts: 4, Rewards: attoFil(4), TotalRewards: attoFil(40)},
			},
		},
	}

	list, all := buildRewardDetails([]chain.SmartAddress{a, b}, minersReward, minerInfo)

	require.Len(t, list, 4)
	require.Equal(t, int64(10), all.BlockCount)
	require.Equal(t, int64(10), all.WinCount)
	require.True(t, decimal.NewFromInt(10).Equal(all.BlockReward))
	// 100 = (10+20) + (30+40)，逐日 total_reward 之和
	require.True(t, decimal.NewFromInt(100).Equal(all.TotalReward))
}
