package chain

import (
	"fmt"
	"github.com/shopspring/decimal"
)

type Ratio decimal.Decimal

func (r Ratio) Percent() string {
	return fmt.Sprintf("%s%%", decimal.Decimal(r).Mul(decimal.NewFromInt(100)).Round(2))
}

func (r Ratio) Decimal() decimal.Decimal {
	return decimal.Decimal(r)
}
