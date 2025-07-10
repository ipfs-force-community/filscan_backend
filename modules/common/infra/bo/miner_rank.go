package bo

import "github.com/shopspring/decimal"

type MinerRank struct {
	Miner                  string
	QualityAdjPower        decimal.Decimal
	QualityAdjPowerPercent decimal.Decimal
	QualityAdjPowerChange  decimal.Decimal
	AccReward              decimal.Decimal
	AccRewardPercent       decimal.Decimal
	AccBlockCount          int64
	AccBlockCountPercent   decimal.Decimal
	Balance                decimal.Decimal
}
