package bo

import (
	"github.com/shopspring/decimal"
)

type BaseLinePower struct {
	Epoch           int64
	Baseline        decimal.Decimal
	RawBytePower    decimal.Decimal
	QualityAdjPower decimal.Decimal
}
