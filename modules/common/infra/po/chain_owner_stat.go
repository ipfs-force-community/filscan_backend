package po

import "github.com/shopspring/decimal"

type OwnerStat struct {
	Epoch                 int64
	Owner                 string
	Interval              string
	PrevEpochRef          int64
	RawBytePowerChange    decimal.Decimal
	QualityAdjPowerChange decimal.Decimal
	SectorCountChange     int64
	SectorPowerChange     decimal.Decimal
	InitialPledgeChange   decimal.Decimal
	AccReward             decimal.Decimal
	AccRewardPercent      decimal.Decimal
	AccBlockCount         int64
	AccBlockCountPercent  decimal.Decimal
	AccWinCount           int64
	AccSealGas            decimal.Decimal
	AccWdPostGas          decimal.Decimal
	RewardPowerRatio      decimal.Decimal
}

func (OwnerStat) TableName() string {
	return "chain.owner_stats"
}
