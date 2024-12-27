package propo

import "github.com/shopspring/decimal"

type MinerAggReward struct {
	Miner       string
	AggReward   decimal.Decimal
	AggBlock    int64
	AggWinCount int64
}

func (MinerAggReward) TableName() string {
	return "pro.miner_agg_rewards"
}
