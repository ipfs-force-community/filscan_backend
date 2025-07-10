package bo

import "github.com/shopspring/decimal"

type NFTOwner struct {
	Owner   string
	Tokens  int64
	Rank    int64
	Percent decimal.Decimal
}
