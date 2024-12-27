package filscan

import (
	"context"

	"github.com/shopspring/decimal"
)

type IMToken interface {
	MessageByEpoch(ctx context.Context, req MessageListRequest) (resp MessageListResponse, err error)
	MessageByCid(ctx context.Context, req MessageByCidRequest) (resp MessageByCidResponse, err error)
	ChainMessages(ctx context.Context, req ChainMessagesRequest) (resp ChainMessagesResponse, err error)
}

type MessageListRequest struct {
	Epoch int    `json:"epoch"`
	Type  string `json:"method_filter"`
}

type MessageListResponse struct {
	MessageList []*MessageIMToken `json:"message_list"`
	//BlockList   map[int][]*BlockIMToken `json:"block_list"`
}

type MessageByCidRequest struct {
	Cid string `json:"cid"`
}

type MessageByCidResponse struct {
	Message *MessageIMToken `json:"message"`
	//BlockList map[int][]*BlockIMToken `json:"block_list"`
}

type ChainMessagesRequest struct {
	Address string `json:"address"`
	RowId   int64  `json:"row_id"`
	Epoch   int64  `json:"epoch"`
}

type ChainMessagesResponse struct {
	MessageList []*MessageIMToken `json:"message_list"`
	//BlockList   map[int][]*BlockIMToken `json:"block_list"`
}

type MessageIMToken struct {
	Cid        string          `json:"cid"`
	From       string          `json:"from"`
	To         string          `json:"to"`
	Nonce      int             `json:"nonce"`
	Value      decimal.Decimal `json:"value"`
	GasFeeCap  decimal.Decimal `json:"gas_fee_cap"`
	GasPremium decimal.Decimal `json:"gas_premium"`
	GasLimit   int64           `json:"gas_limit"`
	Method     string          `json:"method"`
	Exit       int             `json:"exit"`
	GasUsed    decimal.Decimal `json:"gas_used"`
	RowId      int64           `json:"row_id"`
	Epoch      int64           `json:"epoch"`
	BaseFee    decimal.Decimal `json:"base_fee"`
}

type BlockIMToken struct {
	Cid             string          `json:"cid"`
	Epoch           int             `json:"epoch"`
	ParentsWeight   decimal.Decimal `json:"parents_weight"`
	Miner           string          `json:"miner"`
	ParentStateRoot string          `json:"parent_state_root"`
	BlockTime       int             `json:"block_time"`
	BaseFee         decimal.Decimal `json:"base_fee"`
}
