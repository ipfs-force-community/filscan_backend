package probo

import "github.com/shopspring/decimal"

type RichAccountRank struct {
	Actor   string
	Balance decimal.Decimal
	Type    string
}

type RichAccountRankList struct {
	RichAccountRankList []*RichAccountRank
}
