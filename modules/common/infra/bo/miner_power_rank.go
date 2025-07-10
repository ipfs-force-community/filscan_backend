package bo

import "github.com/shopspring/decimal"

type MinerPowerRank struct {
	Epoch                 int64
	PrevEpoch             int64
	Miner                 string
	QualityAdjPowerChange decimal.Decimal
	QualityAdjPower       decimal.Decimal
	RawBytePower          decimal.Decimal
	SectorSize            int64
}
