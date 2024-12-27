package chain

import (
	"github.com/dustin/go-humanize"
	"github.com/shopspring/decimal"
)

type Power decimal.Decimal

func (p Power) Decimal() decimal.Decimal {
	return decimal.Decimal(p)
}

func (p Power) Humanize() string {
	return humanize.BigBytes(decimal.Decimal(p).BigInt())
}

var PerT = decimal.NewFromInt(1024).Pow(decimal.NewFromInt(4))
