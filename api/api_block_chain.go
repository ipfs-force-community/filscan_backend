package filscan

import (
	"context"
	"encoding/json"
	"math/big"

	"github.com/shopspring/decimal"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type BlockChainAPI interface {
	FinalHeight(ctx context.Context, req FinalHeightRequest) (resp FinalHeightResponse, err error)
	TipsetDetail(ctx context.Context, req TipsetDetailRequest) (resp TipsetDetailResponse, err error)
	LatestBlocks(ctx context.Context, req LatestBlocksRequest) (resp LatestBlocksResponse, err error)
	BlockDetails(ctx context.Context, req BlockDetailsRequest) (resp BlockDetailsResponse, err error)
	MessagesByBlock(ctx context.Context, req MessagesByBlockRequest) (resp MessagesByBlockResponse, err error)
	LatestMessages(ctx context.Context, req LatestMessagesRequest) (resp LatestMessagesResponse, err error)
	MessageDetails(ctx context.Context, req MessageDetailsRequest) (resp MessageDetailsResponse, err error)
	LargeTransfers(ctx context.Context, req LargeTransfersRequest) (resp LargeTransfersResponse, err error)
	SearchMarketDeals(ctx context.Context, req SearchMarketDealsRequest) (resp SearchMarketDealsResponse, err error)
	MessagesPool(ctx context.Context, req MessagesPoolRequest) (resp MessagesPoolResponse, err error)
	AllMethods(ctx context.Context, req AllMethodRequest) (resp AllMethodResponse, err error)
	AllMethodsByBlock(ctx context.Context, req AllMethodsByBlockRequest) (resp AllMethodsByBlockResponse, err error)
	AllMethodsByMessagePool(ctx context.Context, req AllMethodsByMessagePoolRequest) (resp AllMethodsByMessagePoolResponse, err error)
	FilPrice(ctx context.Context, req struct{}) (resp *FilPriceResponse, err error)
	GasSummary(ctx context.Context, req struct{}) (resp *GasSummaryResponse, err error)
	TipsetStateTree(ctx context.Context, req TipsetStateTreeRequest) (resp TipsetStateTreeResponse, err error)

	AccountAPI
	RichAccountAPI
}

type RichAccountAPI interface {
	RichAccountRank(ctx context.Context, query PagingQuery) (resp RichAccountsResponse, err error)
}

type DealDetailAPI interface {
	DealDetails(ctx context.Context, req DealDetailsRequest) (resp DealDetailsResponse, err error)
}

// -----------------------区块链页接口参数结构-----------------------

type FinalHeightRequest struct {
}

type FinalHeightResponse struct {
	Height    int64           `json:"height"`
	BlockTime int64           `json:"block_time"`
	BaseFee   decimal.Decimal `json:"base_fee"`
}

type TipsetDetailRequest struct {
	Height int64 `json:"height"`
}

type TipsetDetailResponse struct {
	TipsetDetail *Tipset `json:"tipset_detail"`
}

type LatestBlocksRequest struct {
	Filters types.Filters `json:"filters"`
}

type LatestBlocksResponse struct {
	BlockBasicList []*Tipset `json:"tipset_list"` // 区块基本信息列表
	TotalCount     int64     `json:"total_count"`
}

type BlockDetailsRequest struct {
	BlockCid string `json:"block_cid"` // 区块cid
}

type BlockDetailsResponse struct {
	BlockDetails *BlockDetails `json:"block_details"`
}

type MessagesByBlockRequest struct {
	BlockCid string        `json:"block_cid"` // 区块cid
	Filters  types.Filters `json:"filters"`
}

type MessagesByBlockResponse struct {
	MessageList []*MessageBasic `json:"message_list"` // 消息列表
	TotalCount  int64           `json:"total_count"`
}

type LatestMessagesRequest struct {
	Filters types.Filters `json:"filters"`
}

type LatestMessagesResponse struct {
	MessageList []*MessageBasic `json:"message_list"` // 消息列表
	TotalCount  int64           `json:"total_count"`
}

type MessageDetailsRequest struct {
	MessageCid string `json:"message_cid"` // 消息cid
}

type MessageDetailsResponse struct {
	MessageDetails *MessageDetails // 消息详情
}

type RichAccountsRequest struct {
	Filters types.Filters `json:"filters"`
}

type RichAccountsResponse struct {
	GetRichAccountList []*RichAccount `json:"get_rich_account_list"` // 所有富豪榜账户列表
	TotalCount         int64          `json:"total_count"`           // 总账户数
}

type LargeTransfersRequest struct {
	Filters types.Filters `json:"filters"`
}

type LargeTransfersResponse struct {
	LargeTransferList []*MessageBasic `json:"large_transfer_list"` // 大额转账列表
	TotalCount        int64           `json:"total_count"`         // 总消息数
}

type MarketDealsRequest struct {
	Filters types.Filters `json:"filters"`
}

type MarketDealsResponse struct {
	MarketDealsList []*MarketDeal `json:"market_deals_list"` // 订单列表
	TotalCount      int64         `json:"total_count"`       // 订单列表总数
}

type SearchMarketDealsRequest struct {
	Input      string        `json:"input"`
	IsVerified *bool         `json:"is_verified"`
	Filters    types.Filters `json:"filters"`
}

type SearchMarketDealsResponse struct {
	MarketDealsList []*MarketDeal `json:"market_deals_list"` // 订单列表
	TotalCount      int64         `json:"total_count"`       // 订单列表总数
}

type DealDetailsRequest struct {
	DealID int64 `json:"deal_id"` // 交易
}

type DealDetailsResponse struct {
	DealDetails *DealDetails `json:"deal_details"`
}

type MessagesPoolRequest struct {
	Cid     string        `json:"cid"`
	Filters types.Filters `json:"filters"`
}

type MessagesPoolResponse struct {
	MessagesPoolList []*MessagePool `json:"messages_pool_list"` // 消息池列表
	TotalCount       int64          `json:"total_count"`        // 消息池列表总数
}

type AllMethodRequest struct {
}
type TipsetStateTreeRequest struct {
	Filters types.TipsetFilters `json:"filters"`
}

type AllMethodResponse struct {
	MethodNameList map[string]int64 `json:"method_name_list"`
}

type AllMethodsByBlockRequest struct {
	Cid string `json:"cid"`
}

type AllMethodsByBlockResponse struct {
	MethodNameList map[string]int64 `json:"method_name_list"`
}
type AllMethodsByMessagePoolRequest struct {
}

type AllMethodsByMessagePoolResponse struct {
	MethodNameList map[string]int64 `json:"method_name_list"`
}

// -----------------------区块链页基础数据结构-----------------------

type BlockBasic struct {
	Height        *big.Int         `json:"height"`                  // 当前高度
	Cid           string           `json:"cid"`                     // 当前区块cid
	BlockTime     uint64           `json:"block_time"`              // 当前区块时间
	MinerID       string           `json:"miner_id"`                // 赢票节点号
	MessagesCount int64            `json:"messages_count"`          // 区块消息数
	Reward        decimal.Decimal  `json:"reward"`                  // 奖励
	MinedReward   *decimal.Decimal `json:"mined_reward,omitempty"`  // 区块奖励
	TxFeeReward   *decimal.Decimal `json:"tx_fee_reward,omitempty"` // 区块奖励手续费
}

type Tipset struct {
	Height                  *big.Int      `json:"height"`
	BlockTime               uint64        `json:"blcok_time,omitempty"`
	MessageCountDeduplicate int64         `json:"message_count_deduplicate,omitempty"`
	BlockBasic              []*BlockBasic `json:"block_basic"`
	MinTicketBlock          string        `json:"min_ticket_block,omitempty"` // 最小赢票值区块
}

type BlockDetails struct {
	BlockBasic    BlockBasic      `json:"block_basic"`     // 区块基础信息
	WinCount      int64           `json:"win_count"`       // 赢票数
	ParentCids    []string        `json:"parents"`         // 父区块cid列表(上一高度的赢票区块cid列表)
	ParentWeight  decimal.Decimal `json:"parent_weight"`   // 父块重量
	ParentBaseFee decimal.Decimal `json:"parent_base_fee"` // 父块基础费率
	TicketValue   string          `json:"ticket_value"`    // 赢票值
	StateRoot     string          `json:"state_root"`      // 根
}

type MessageBasic struct {
	Height     *big.Int        `json:"height"`                // 高度
	BlockTime  uint64          `json:"block_time"`            // 区块时间
	Cid        string          `json:"cid"`                   // 消息cid
	From       string          `json:"from"`                  // 发送地址
	To         string          `json:"to"`                    // 接收地址
	Value      decimal.Decimal `json:"value"`                 // 数额
	ExitCode   string          `json:"exit_code,omitempty"`   // 状态码
	MethodName string          `json:"method_name,omitempty"` // 方法名称
	FromTag    string          `json:"from_tag"`
	ToTag      string          `json:"to_tag"`
}

func (c MessageBasic) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Height     *big.Int        `json:"height"`                // 高度
		BlockTime  uint64          `json:"block_time"`            // 区块时间
		Cid        string          `json:"cid"`                   // 消息cid
		From       string          `json:"from"`                  // 发送地址
		To         string          `json:"to"`                    // 接收地址
		Value      decimal.Decimal `json:"value"`                 // 数额
		ExitCode   string          `json:"exit_code,omitempty"`   // 状态码
		MethodName string          `json:"method_name,omitempty"` // 方法名称
		FromTag    string          `json:"from_tag"`
		ToTag      string          `json:"to_tag"`
	}{
		Height:     c.Height,
		BlockTime:  c.BlockTime,
		Cid:        c.Cid,
		From:       c.From,
		To:         c.To,
		Value:      c.Value,
		ExitCode:   c.ExitCode,
		MethodName: c.MethodName,
		FromTag:    convertor.GlobalTagMap[c.From],
		ToTag:      convertor.GlobalTagMap[c.To],
	})
}

type MessageDetails struct {
	MessageBasic MessageBasic    `json:"message_basic"`          // 消息基础信息
	BlkCids      []string        `json:"blk_cids"`               // 所在区块cid列表
	ConsumeList  []*Consume      `json:"consume_list,omitempty"` // 转账信息列表
	Version      int             `json:"version"`                // 版本编号
	Nonce        uint64          `json:"nonce"`                  // Nonce数
	GasFeeCap    decimal.Decimal `json:"gas_fee_cap"`            // 手续费上限
	GasPremium   decimal.Decimal `json:"gas_premium"`            // 节点小费费率
	GasLimit     int64           `json:"gas_limit"`              // Gas用量上限
	GasUsed      decimal.Decimal `json:"gas_used"`               // Gas实际用量
	BaseFee      decimal.Decimal `json:"base_fee"`               // 基础手续费
	AllGasFee    decimal.Decimal `json:"all_gas_fee"`            // 总手续费
	//Args         json.RawMessage     `json:"args"`          // 消息参数
	ParamsDetail  interface{} `json:"params_detail"`  // 已解析/未解析的参数
	ReturnsDetail interface{} `json:"returns_detail"` // 已解析/未解析的返回值
	ETHMessage    string      `json:"eth_message"`    // ETH消息
	Error         string      `json:"err"`            //错误信息(ExitCode为0时为空)
	Replaced      bool        `json:"replaced"`       //消息是否被覆盖
}

type SubtypeData struct {
	Subtype int    `json:"Subtype"`
	Data    string `json:"Data"`
}

type RichAccount struct {
	AccountID          string          `json:"account_id"`           // 账户ID
	AccountAddress     string          `json:"account_address"`      // 账户地址
	AccountType        string          `json:"account_type"`         // 账户类型
	Balance            decimal.Decimal `json:"balance"`              // 余额
	BalancePercentage  decimal.Decimal `json:"balance_percentage"`   // 余额占比
	LatestTransferTime int64           `json:"latest_transfer_time"` // 最新交易时间
}

type MarketDeal struct {
	DealID                int64           `json:"deal_id"`                  // 交易ID
	PieceCid              string          `json:"piece_cid"`                // 文件ID
	PieceSize             decimal.Decimal `json:"piece_size"`               // 文件大小
	ClientAddress         string          `json:"client_address"`           // 客户地址
	ProviderID            string          `json:"provider_id"`              // 托管者ID
	StartHeight           *big.Int        `json:"start_height"`             // 开始高度
	StartTime             int64           `json:"service_start_time"`       // 开始时间
	EndHeight             *big.Int        `json:"end_height"`               // 结束高度
	EndTime               int64           `json:"end_time"`                 // 结束时间
	StoragePricePerHeight decimal.Decimal `json:"storage_price_per_height"` // 每高度的存储费用
	VerifiedDeal          bool            `json:"verified_deal"`            // 是否已验证
	Label                 interface{}     `json:"label"`                    // 标签
}

type DealDetails struct {
	DealID               int64           `json:"deal_id"`                 // 交易ID
	Epoch                int64           `json:"epoch"`                   // 所属区块高度
	MessageCid           string          `json:"message_cid"`             // 所属消息Cid
	PieceCid             string          `json:"piece_cid"`               // 文件ID
	VerifiedDeal         bool            `json:"verified_deal"`           // 是否已验证
	PieceSize            int64           `json:"piece_size"`              // 文件大小
	Client               string          `json:"client_id"`               // 客户ID或地址
	ClientCollateral     decimal.Decimal `json:"client_collateral"`       // 客户质押金额
	Provider             string          `json:"provider_id"`             // 托管者ID或地址
	ProviderCollateral   decimal.Decimal `json:"provider_collateral"`     // 托管者质押金额
	StartEpoch           int64           `json:"start_epoch"`             // 开始高度
	StartTime            int64           `json:"service_start_time"`      // 开始时间
	EndEpoch             int64           `json:"end_epoch"`               // 结束高度
	EndTime              int64           `json:"end_time"`                // 结束时间
	StoragePricePerEpoch decimal.Decimal `json:"storage_price_per_epoch"` // 托管费用
	Label                interface{}     `json:"label"`                   // 标签
}

type MessagePool struct {
	MessageBasic MessageBasic    `json:"message_basic"` // 消息基础信息
	GasLimit     int64           `json:"gas_limit"`     // 手续费上限
	GasPremium   decimal.Decimal `json:"gas_premium"`   // 节点小费费率
}

type FilPriceResponse struct {
	Price            float64 `json:"price"`
	PercentChange24h float64 `json:"percent_change_24h"`
}

type GasSummaryResponse struct {
	Sum         decimal.Decimal `json:"sum"`
	ContractGas decimal.Decimal `json:"contract_gas"`
	Others      decimal.Decimal `json:"others"`
}

type TipsetState struct {
	Height       int64
	OrphanBlocks []londobell.BlockHeader
	ChainBlocks  []londobell.BlockHeader
}
type TipsetStateTreeResponse struct {
	TipsetList []TipsetState `json:"tipset_list"`
}
