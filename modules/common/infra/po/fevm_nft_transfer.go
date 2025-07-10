package po

import "github.com/shopspring/decimal"

type NFTTransfer struct {
	Epoch    int64
	Cid      string
	Contract string
	From     string
	To       string
	Method   string
	TokenId  string
	Item     string
	Value    decimal.Decimal
}

func (NFTTransfer) TableName() string {
	return "fevm.nft_transfers"
}
