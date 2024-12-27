package po

import "github.com/shopspring/decimal"

type FEvmERC20Transfer struct {
	Epoch      int64
	Cid        string
	ContractId string
	From       string
	To         string
	Amount     decimal.Decimal
	DEX        string `gorm:"column:dex"`
	Method     string
	TokenName  string
	Decimal    int
	Index      int
}

func (FEvmERC20Transfer) TableName() string {
	return "fevm.erc_20_transfers"
}
