package po

import "github.com/shopspring/decimal"

type MinerRewardStat struct {
	Epoch         int64
	Interval      string
	AccReward     decimal.Decimal
	AccRewardPerT decimal.Decimal
}

func (MinerRewardStat) TableName() string {
	return "chain.miner_reward_stats"
}
