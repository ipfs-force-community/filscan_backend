package filscan

import (
	"context"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type IndexAPI interface {
	TotalIndicators(ctx context.Context, req TotalIndicatorsRequest) (resp *TotalIndicatorsResponse, err error)
	BannerIndicator(ctx context.Context, req struct{}) (resp *BannerIndicatorsResponse, err error)
	SearchInfo(ctx context.Context, req SearchInfoRequest) (resp SearchInfoResponse, err error)
	StatisticAPI
	RankAPI
}

// -----------------------首页接口参数结构-----------------------

type TotalIndicatorsRequest struct {
}

type TotalIndicatorsResponse struct {
	TotalIndicators TotalIndicators `json:"total_indicators"` // 首页指标
}

type BannerIndicatorsResponse struct {
	TotalBalance  *decimal.Decimal `json:"total_balance"`
	Proportion32G *decimal.Decimal `json:"proportion_32G"`
	Proportion64G *decimal.Decimal `json:"proportion_64G"`
}

type SearchInfoRequest struct {
	Input     string          `json:"input"`
	InputType types.InputType `json:"input_type"`
}

type SearchInfoResponse struct {
	Epoch      int64             `json:"epoch"`
	ResultType string            `json:"result_type"`
	FNSTokens  []*SearchFNSToken `json:"fns_tokens,omitempty"`
}

type SearchFNSToken struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Icon     string `json:"icon"`
}

// -----------------------首页基础数据结构结构-----------------------

type TotalIndicators struct {
	LatestHeight       int64           `json:"latest_height"`       // 最新区块高度
	LatestBlockTime    int64           `json:"latest_block_time"`   // 最新区块时间
	TotalBlocks        int64           `json:"total_blocks"`        // 全网出块数量
	TotalRewards       decimal.Decimal `json:"total_rewards"`       // 全网出块奖励，单位Fil
	TotalQualityPower  decimal.Decimal `json:"total_quality_power"` // 全网有效算力
	Dc                 decimal.Decimal // DC 算力
	Cc                 decimal.Decimal // CC 算力
	BaseFee            decimal.Decimal `json:"base_fee"`             // 当前基础费率
	MinerInitialPledge decimal.Decimal `json:"miner_initial_pledge"` // 当前扇区质押量
	PowerIncrease24H   decimal.Decimal `json:"power_increase_24h"`   // 近24h增长算力
	RewardsIncrease24H decimal.Decimal `json:"rewards_increase_24h"` // 近24h出块奖励
	FilPerTera24H      decimal.Decimal `json:"fil_per_tera_24h"`     // 近24h产出效率，单位Fil/T
	GasIn32G           decimal.Decimal `json:"gas_in_32g"`           // 32GiB扇区Gas消耗，单位Fil/T
	AddPowerIn32G      decimal.Decimal `json:"add_power_in_32g"`     // 32GiB扇区新增算力成本，单位Fil/T
	GasIn64G           decimal.Decimal `json:"gas_in_64g"`           // 64GiB扇区Gas消耗，单位Fil/T
	AddPowerIn64G      decimal.Decimal `json:"add_power_in_64g"`     // 64GiB扇区新增算力成本，单位Fil/T
	WinCountReward     decimal.Decimal `json:"win_count_reward"`     // 每赢票奖励，单位Fil
	AvgBlockCount      decimal.Decimal `json:"avg_block_count"`      // 平均每高度区块数量
	AvgMessageCount    float64         `json:"avg_message_count"`    // 平均每高度消息数
	ActiveMiners       int64           `json:"active_miners"`        // 活跃节点数
	Burnt              decimal.Decimal `json:"burnt"`                // 销毁量
	CirculatingPercent decimal.Decimal `json:"circulating_percent"`  // 流通率
	Sum                decimal.Decimal `json:"sum"`
	ContractGas        decimal.Decimal `json:"contract_gas"`
	Others             decimal.Decimal `json:"others"`
}
