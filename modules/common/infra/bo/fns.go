package bo

import "github.com/shopspring/decimal"

type FnsSummary struct {
	Tokens      int64
	Controllers int64
	Transfers   int64
}

type FnsOwnerToken struct {
	Name     string
	Provider string
}

type FnsRegistrant struct {
	Registrant string
	Tokens     int64
	Rank       int64
	Percent    decimal.Decimal
}
