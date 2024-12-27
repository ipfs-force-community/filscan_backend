package bo

import "github.com/shopspring/decimal"

type DCPower struct {
	Epoch int64
	Dc    decimal.Decimal
	Cc    decimal.Decimal
}
