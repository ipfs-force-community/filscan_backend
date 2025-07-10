package po

import (
	"github.com/shopspring/decimal"
)

type BaseLinePowerPo struct {
	Epoch           int64
	Baseline        decimal.Decimal
	RawBytePower    decimal.Decimal
	QualityAdjPower decimal.Decimal
}

func (BaseLinePowerPo) TableName() string {
	return "chain.base_line_powers"
}
