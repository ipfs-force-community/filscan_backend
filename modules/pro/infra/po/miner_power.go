package propo

import "github.com/shopspring/decimal"

type MinerDcPower struct {
	Epoch int64
	Miner string
	Vdc   decimal.Decimal
	Dc    decimal.Decimal
	Cc    decimal.Decimal
}

func (MinerDcPower) TableName() string {
	return "pro.miner_dc_powers"
}
