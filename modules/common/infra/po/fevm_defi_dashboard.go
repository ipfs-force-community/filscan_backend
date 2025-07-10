package po

import "github.com/shopspring/decimal"

type DefiDashboard struct {
	Id         int
	Epoch      int
	Protocol   string
	ContractId string
	Tvl        decimal.Decimal
	TvlInFil   decimal.Decimal
	Users      int
	Url        string
}

func (DefiDashboard) TableName() string {
	return "fevm.defi_dashboard"
}
