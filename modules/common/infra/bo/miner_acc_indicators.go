package bo

import (
	"github.com/shopspring/decimal"
)

type AccIndicators struct {
	AccBlockCount int64
	AccReward     decimal.Decimal
	AccWinCount   int64
	AccSealGas    decimal.Decimal
	AccWdPostGas  decimal.Decimal
}
