package propo

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type MinerInfo struct {
	Epoch           int64
	Miner           string
	Owner           string
	Worker          string
	Controllers     types.StringArray
	Beneficiary     string
	RawBytePower    decimal.Decimal
	QualityAdjPower decimal.Decimal
	Pledge          decimal.Decimal
	LiveSectors     int64
	ActiveSectors   int64
	FaultSectors    int64
	SectorSize      int64
	Padding         bool
}

func (MinerInfo) TableName() string {
	return "pro.miner_infos"
}

type MinerDc struct {
	Epoch           int64
	Miner           string
	RawBytePower    decimal.Decimal
	QualityAdjPower decimal.Decimal
	Pledge          decimal.Decimal
	LiveSectors     int64
	ActiveSectors   int64
	FaultSectors    int64
	SectorSize      int64
	VdcPower        decimal.Decimal
	DcPower         decimal.Decimal
	CCPower         decimal.Decimal
}

func (MinerDc) TableName() string {
	return "pro.miner_dcs"
}
