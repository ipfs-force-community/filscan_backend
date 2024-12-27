package bo

import "github.com/shopspring/decimal"

type RichAccountRank struct {
	Epoch   int64
	Actor   string
	Balance decimal.Decimal
	Type    string
}

type RichAccountRankList struct {
	RichAccountRankList []*RichAccountRank
	TotalCount          int64
}
