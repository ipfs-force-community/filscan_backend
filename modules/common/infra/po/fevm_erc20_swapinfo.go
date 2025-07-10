package po

import "github.com/shopspring/decimal"

type FEvmERC20SwapInfo struct {
	Cid                 string
	Action              string
	Epoch               int
	AmountIn            decimal.Decimal
	AmountOut           decimal.Decimal
	AmountInTokenName   string
	AmountInContractId  string
	AmountOutTokenName  string
	AmountOutContractId string
	AmountInDecimal     int
	AmountOutDecimal    int
	Dex                 string
	SwapRate            decimal.Decimal
	Values              decimal.Decimal
}

func (FEvmERC20SwapInfo) TableName() string {
	return "fevm.erc20_swap_info"
}
