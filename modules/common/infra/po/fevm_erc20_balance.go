package po

import "github.com/shopspring/decimal"

type FEvmERC20Balance struct {
	Owner      string
	ContractId string
	Amount     decimal.Decimal
}

func (FEvmERC20Balance) TableName() string {
	return "fevm.erc20_balance"
}
