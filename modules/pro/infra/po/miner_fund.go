package propo

import "github.com/shopspring/decimal"

type MinerFund struct {
	Epoch      int64
	Miner      string
	Income     decimal.Decimal
	Outlay     decimal.Decimal
	TotalGas   decimal.Decimal
	SealGas    decimal.Decimal
	DealGas    decimal.Decimal
	WdPostGas  decimal.Decimal
	Penalty    decimal.Decimal
	Reward     decimal.Decimal
	BlockCount int64
	WinCount   int64
	OtherGas   decimal.Decimal
	PreAgg     decimal.Decimal
	ProAgg     decimal.Decimal
}

func (MinerFund) TableName() string {
	return "pro.miner_funds"
}
