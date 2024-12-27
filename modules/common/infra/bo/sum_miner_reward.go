package bo

import "github.com/shopspring/decimal"

type SumMinerReward struct {
	Epoch         int64
	Balance       decimal.Decimal
	AccRewardPerT decimal.Decimal
}
