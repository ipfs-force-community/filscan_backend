package po

import "github.com/shopspring/decimal"

type AbsPowerChange struct {
	Id            string
	Epoch         int64
	PowerIncrease decimal.Decimal
	PowerLoss     decimal.Decimal
}

func (AbsPowerChange) TableName() string {
	return "chain.abs_power_change"
}
