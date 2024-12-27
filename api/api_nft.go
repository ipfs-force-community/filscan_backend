package filscan

import (
	"context"

	"github.com/shopspring/decimal"
)

type NFTAPI interface {
	NFTTokens(ctx context.Context, request NFTTokensRequest) (reply *NFTTokensReply, err error)
	NFTSummary(ctx context.Context, request NFTSummaryRequest) (reply *NFTSummaryReply, err error)
	NFTOwners(ctx context.Context, request NFTOwnersRequest) (reply *NFTOwnersReply, err error)
	NFTTransfers(ctx context.Context, request *NFTTransfersRequest) (reply *NFTTransfersReply, err error)
	NFTMessageTransfers(ctx context.Context, request NFTMessageTransfersRequest) (reply *NFTMessageTransfersReply, err error)
}

type NFTTokensRequest struct {
	Index int `json:"index"`
	Limit int `json:"limit"`
}

type NFTTokensReply struct {
	Total int64       `json:"total"`
	Items []*NFTToken `json:"items"`
}

type NFTToken struct {
	Icon          string          `json:"icon"`
	Collection    string          `json:"collection"`
	TradingVolume decimal.Decimal `json:"trading_volume"`
	Holders       int64           `json:"holders"`
	Transfers     int64           `json:"transfers"`
	Provider      string          `json:"provider"`
	Contract      string          `json:"contract"`
	Mints         int64           `json:"mints"`
}

type NFTMessageTransfersRequest struct {
	Cid string `json:"cid"`
}

type NFTMessageTransfersReply struct {
	Items []*NFTMessageTransfersReplyItem `json:"items"`
}

type NFTMessageTransfersReplyItem struct {
	Cid       string          `json:"cid"`
	Method    string          `json:"method"`
	Time      int64           `json:"time"`
	From      string          `json:"from"`
	To        string          `json:"to"`
	Amount    decimal.Decimal `json:"amount"`
	TokenName string          `json:"token_name"`
	Contract  string          `json:"contract"`
	Provider  string          `json:"provider"`
	Item      string          `json:"item"`
	Url       string          `json:"url"`
}

type NFTSummaryRequest struct {
	Contract string `json:"contract"`
}

type NFTSummaryReply struct {
	TotalSupply int64  `json:"total_supply"`
	Owners      int64  `json:"owners"`
	Transfers   int64  `json:"transfers"`
	Contract    string `json:"contract"`
	TokenName   string `json:"token_name"`
	Logo        string `json:"logo"`
	MainSite    string `json:"main_site"`
	TwitterLink string `json:"twitter_link"`
}

type NFTTransfersRequest struct {
	Contract string `json:"contract"`
	Index    int    `json:"page"`
	Limit    int    `json:"limit"`
}

type NFTTransfersReply struct {
	Total int64          `json:"total"`
	Items []*NFTTransfer `json:"items"`
}

type NFTTransfer struct {
	Cid    string          `json:"cid"`
	Method string          `json:"method"`
	Time   int64           `json:"time"`
	From   string          `json:"from"`
	To     string          `json:"to"`
	Item   string          `json:"item"`
	Value  decimal.Decimal `json:"value"`
	Url    string          `json:"url"`
}

type NFTOwnersRequest struct {
	Contract string `json:"contract"`
	Index    int    `json:"page"`
	Limit    int    `json:"limit"`
}

type NFTOwnersReply struct {
	Total int64       `json:"total"`
	Items []*NFTOwner `json:"items"`
}

type NFTOwner struct {
	Rank       int64           `json:"rank"`
	Owner      string          `json:"owner"`
	Amount     int64           `json:"amount"`
	Percentage decimal.Decimal `json:"percentage"`
}
