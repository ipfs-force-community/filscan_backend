package po

import "github.com/shopspring/decimal"

type FEvmERC20Contract struct {
	ContractId  string
	TotalSupply decimal.Decimal
	Pair        string
	TokenName   string
	Decimal     int
	TwitterLink string
	MainSite    string
	Url         string
}

func (FEvmERC20Contract) TableName() string {
	return "fevm.erc20_contract"
}
