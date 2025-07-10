package filscan

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
)

type ERC20API interface {
	ERC20Summary(ctx context.Context, contractID *ERC20SummaryRequest) (*ERC20SummaryReply, error)
	ERC20Transfer(ctx context.Context, request *ERC20TransferReq) (*ERC20TransferReply, error)
	ERC20TransferInMessage(ctx context.Context, request *ERC20TransferInMessageReq) (*ERC20TransferInMessageReply, error)
	ERC20Market(ctx context.Context, contractID *ERC20SummaryRequest) (*ERC20MarketReply, error)
	ERC20List(ctx context.Context, _ struct{}) (*ERC20ContractsReply, error)
	ERC20Owner(ctx context.Context, request *ERC20TransferReq) (*ERC20OwnerListReply, error)
	ERC20DexTrade(ctx context.Context, request *ERC20TransferReq) (*ERC20DexTradeListReply, error)
	// TODO: should remove from here
	EventsInMessage(ctx context.Context, request *EventsInMessageReq) (*EventsInMessageReply, error)
	InternalTransfer(ctx context.Context, request *InternalTransferReq) (*InternalTransferReply, error)
	SwapInfoInMessage(ctx context.Context, request *ERC20TransferInMessageReq) (*SwapInfoReply, error)

	ERC20TokenHolder(ctx context.Context, request *ERC20TokenHolderRequest) (*ERC20TokenHolderReply, error)
	ERC20OwnerTokenList(ctx context.Context, request *ERC20HolderRequest) (*ERC20HolderReply, error)
	ERC20AddrTransfers(ctx context.Context, request *ERC20AddrTransfersReq) (*ERC20AddrTransfersReply, error)

	ERC20RecentTransfer(ctx context.Context, request *ERC20RecentTransferReq) (*ERC20TransferReply, error)
}

type ERC20RecentTransferReq struct {
	Contract
	Duration string `json:"duration"`
	Page     int64  `json:"page"`
	Limit    int64  `json:"limit"`
}

type Contract struct {
	ContractID string `json:"contract_id"`
}

type ERC20AddrTransfersReq struct {
	Address string `json:"address"`
	Filters struct {
		Page  int64 `json:"page"`
		Limit int64 `json:"limit"`
	} `json:"filters"`
	TokenName string `json:"token_name"`
}

type ERC20AddrTransfersReply struct {
	Total int64            `json:"total"`
	Items []*ERC20Transfer `json:"items"`
}

type ERC20AddrTransfersTokenTypesReq struct {
	Address string `json:"address"`
}

type ERC20AddrTransfersTokenTypesReply struct {
	TokenNames []string `json:"token_names"`
}

type ERC20TokenHolderRequest struct {
	Address    string `json:"address"`
	ContractID string `json:"contract_id"`
}

type ERC20HolderRequest struct {
	Address string `json:"address"`
	Filters struct {
		Page  int64 `json:"page"`
		Limit int64 `json:"limit"`
	} `json:"filters"`
}

type ERC20HolderItem struct {
	ContractID string          `json:"contract_id"`
	TokenName  string          `json:"token_name"`
	Amount     decimal.Decimal `json:"amount"`
	Value      decimal.Decimal `json:"value"`
	IconUrl    string          `json:"icon_url"`
}

type ERC20HolderReply struct {
	Total      int               `json:"total"`
	Items      []ERC20HolderItem `json:"items"`
	TotalValue decimal.Decimal   `json:"total_value"`
}

type ERC20TokenHolderReply struct {
	Amount  decimal.Decimal `json:"amount"`
	Decimal int             `json:"decimal"`
}

type ERC20SummaryRequest struct {
	Contract
}

type ERC20SummaryReply struct {
	TotalSupply decimal.Decimal `json:"total_supply"`
	Owners      int64           `json:"owners"`
	Transfers   int64           `json:"transfers"`
	ContractID  string          `json:"contract_id"`
	TokenName   string          `json:"token_name"`
	TwitterLink string          `json:"twitter_link"`
	MainSite    string          `json:"main_site"`
	IconUrl     string          `json:"icon_url"`
}

type ERC20MarketReply struct {
	ContractID  string `json:"contract_id"`
	LatestPrice string `json:"latest_price"`
	MarketCap   string `json:"market_cap"`
	TokenName   string `json:"token_name"`
}

type ERC20TransferReq struct {
	Contract
	Page   int64  `json:"page"`
	Limit  int64  `json:"limit"`
	Filter string `json:"filter"`
}

type ERC20TransferReply struct {
	Total int64            `json:"total"`
	Items []*ERC20Transfer `json:"items"`
}

type ERC20Owner struct {
	Owner  string          `json:"owner"`
	Rank   int             `json:"rank"`
	Amount decimal.Decimal `json:"amount"`
	Rate   decimal.Decimal `json:"rate"`
	Value  decimal.Decimal `json:"value"`
}

type ERC20OwnerListReply struct {
	Total int64         `json:"total"`
	Items []*ERC20Owner `json:"items"`
}

type ERC20DexInfo struct {
	Cid                string           `json:"cid"`
	Action             string           `json:"action"`
	Time               time.Time        `json:"time"`
	AmountIn           decimal.Decimal  `json:"amount_in"`
	AmountOut          decimal.Decimal  `json:"amount_out"`
	AmountInTokenName  string           `json:"amount_in_token_name"`
	AmountOutTokenName string           `json:"amount_out_token_name"`
	Dex                string           `json:"dex"`
	SwapRate           *decimal.Decimal `json:"swap_rate"`
	Value              *decimal.Decimal `json:"value"`
	SwapTokenName      string           `json:"swap_token_name"`
	DexUrl             string           `json:"dex_url"`
	IconUrl            string           `json:"icon_url"`
}

type ERC20DexTradeListReply struct {
	Total int64           `json:"total"`
	Items []*ERC20DexInfo `json:"items"`
}

type ERC20Transfer struct {
	Cid        string          `json:"cid"`
	Method     string          `json:"method"`
	Time       time.Time       `json:"time"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	Amount     decimal.Decimal `json:"amount"`
	TokenName  string          `json:"token_name"`
	ContractID string          `json:"contract_id"`
	IconUrl    string          `json:"icon_url"`
}

type ERC20TransferInMessageReq struct {
	Cid string `json:"cid"`
}

type ERC20TransferInMessageReply struct {
	Items []*ERC20Transfer `json:"items"`
}

type EventsInMessageReq = ERC20TransferInMessageReq

type EventsLog struct {
	Address  string   `json:"address"`
	Name     *string  `json:"name"`
	Topics   []string `json:"topics"`
	Data     string   `json:"data"`
	LogIndex string   `json:"log_index"`
	Removed  bool     `json:"removed"`
}

type EventsInMessageReply struct {
	Logs []EventsLog `json:"logs"`
}

type InternalTransferReq = ERC20TransferInMessageReq

type InternalTransfer struct {
	Method string          `json:"method"`
	From   string          `json:"from"`
	To     string          `json:"to"`
	Value  decimal.Decimal `json:"value"`
}

type InternalTransferReply struct {
	InternalTransfers []InternalTransfer `json:"internal_transfers"`
}

type SwapInfo struct {
	AmountIn           decimal.Decimal `json:"amount_in"`
	AmountOut          decimal.Decimal `json:"amount_out"`
	AmountInTokenName  string          `json:"amount_in_token_name"`
	AmountOutTokenName string          `json:"amount_out_token_name"`
	Dex                string          `json:"dex"`
	DexUrl             string          `json:"dex_url"`
	IconUrl            string          `json:"icon_url"`
}

type SwapInfoReply struct {
	SwapInfo *SwapInfo `json:"swap_info"`
}

type ERC20Contract struct {
	TokenName   string          `json:"token_name"`
	TotalSupply decimal.Decimal `json:"total_supply"`
	Owners      int64           `json:"owners"`
	ContractID  string          `json:"contract_id"`
	LatestPrice string          `json:"latest_price"`
	MarketCap   string          `json:"market_cap"`
	Vol24       string          `json:"vol_24"`
	IconUrl     string          `json:"icon_url"`
}

type ERC20ContractsReply struct {
	Items []*ERC20Contract `json:"items"`
}
