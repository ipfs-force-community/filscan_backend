package po

import "github.com/shopspring/decimal"

type FEvmErc20Transfer struct {
	Epoch      int64
	Cid        string
	ContractId string
	From       string
	To         string
	Amount     decimal.Decimal
}

func (FEvmErc20Transfer) TableName() string {
	return "fevm.erc_20_transfers"
}
