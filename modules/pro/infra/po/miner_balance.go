package propo

import (
	"github.com/shopspring/decimal"
)

type MinerBalance struct {
	Epoch   int64
	Miner   string
	Type    string
	Address string
	Balance decimal.Decimal
}

func (MinerBalance) TableName() string {
	return "pro.miner_balances"
}
