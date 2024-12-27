package propo

import "github.com/shopspring/decimal"

type MinerSector struct {
	Epoch     int64
	Miner     string
	HourEpoch int64
	Sectors   int64
	Power     decimal.Decimal
	Pledge    decimal.Decimal
	Vdc       decimal.Decimal
	Dc        decimal.Decimal
	Cc        decimal.Decimal
}

func (MinerSector) TableName() string {
	return "pro.miner_sectors"
}
