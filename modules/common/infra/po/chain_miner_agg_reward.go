package po

import "github.com/shopspring/decimal"

type MinerAggReward struct {
	Miner         string
	AggBlockCount int64
	AggWinCount   int64
	AggReward     decimal.Decimal
}

func (MinerAggReward) TableName() string {
	return "chain.miner_age_rewards"
}
