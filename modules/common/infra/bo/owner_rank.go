package bo

import "github.com/shopspring/decimal"

type OwnerRank struct {
	Epoch                 int64
	Owner                 string
	QualityAdjPower       decimal.Decimal
	AccReward             decimal.Decimal
	AccBlockCount         int64
	RewardPowerRatio      decimal.Decimal
	QualityAdjPowerChange decimal.Decimal
}
