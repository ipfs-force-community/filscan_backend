package bo

import "github.com/shopspring/decimal"

type DefiTvl struct {
	Epoch int64
	Tvl   decimal.Decimal
}
