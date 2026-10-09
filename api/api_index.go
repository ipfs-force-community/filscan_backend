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
	LatestHeight      int64           `json:"latest_height"`       // 最新区块高度
	LatestBlockTime   int64           `json:"latest_block_time"`   // 最新区块时间
	TotalBlocks       int64           `json:"total_blocks"`        // 全网出块数量
	TotalRewards      decimal.Decimal `json:"total_rewards"`       // 全网出块奖励，单位Fil
	TotalQualityPower decimal.Decimal `json:"total_quality_power"` // 全网有效算力
	// 全网算力倍数结构（NV29/FIP-0118 方案 A）：口径 chain.QualityTierSplit，两时代同式。
	FullMultiplierPower decimal.Decimal `json:"full_multiplier_power"` // 满倍率算力（处于 10× 档的等效原始字节）
	PendingUpgradePower decimal.Decimal `json:"pending_upgrade_power"` // 待升级算力（未达满倍率的等效原始字节）
	BaseFee             decimal.Decimal `json:"base_fee"`              // 当前基础费率
	MinerInitialPledge  decimal.Decimal `json:"miner_initial_pledge"`  // 当前扇区质押量
	PowerIncrease24H    decimal.Decimal `json:"power_increase_24h"`    // 近24h增长算力
	RewardsIncrease24H  decimal.Decimal `json:"rewards_increase_24h"`  // 近24h出块奖励
	FilPerTera24H       decimal.Decimal `json:"fil_per_tera_24h"`      // 近24h产出效率，单位Fil/T
	// 近24h区块奖励三流拆分（NV29/FIP-0118，attoFIL 原始计数器差分；fil_per_tera_24h 仍为单数字不拆）。
	// 口径见 acl.RewardStreamDeltas24H；取数失败时四者置 0。
	RewardStreamMiner24H   decimal.Decimal `json:"reward_stream_miner_24h"`   // 近24h共识流＝矿工实收
	RewardStreamService24H decimal.Decimal `json:"reward_stream_service_24h"` // 近24h服务流
	RewardStreamBurn24H    decimal.Decimal `json:"reward_stream_burn_24h"`    // 近24h销毁
	RewardStreamTotal24H   decimal.Decimal `json:"reward_stream_total_24h"`   // 近24h铸造量合计
	// NV29(FIP-0118) 累计奖励三股 + 累计铸造量（attoFIL，f02 原始计数器累计值，非 24h 差分）。
	// 口径见 acl.GetRewardStreamTotals：v18 时 minted=miner（否则前端显示 0 与矿工行矛盾）、
	// service/burn 恒 0；v19 时 minted=TotalMintedReward、service=TotalExplicitMinted、burn=TotalBurnMinted。
	// 取数失败时四者置 0（首页不 500）。
	RewardStreamMintedTotal  decimal.Decimal `json:"reward_stream_minted_total"`  // 累计铸造量（三股之和）
	RewardStreamMinerTotal   decimal.Decimal `json:"reward_stream_miner_total"`   // 累计矿工实收
	RewardStreamServiceTotal decimal.Decimal `json:"reward_stream_service_total"` // 累计服务流（记在 f02、待受益方提取）
	// ⚠️ 与上面的 burnt(f099 账户余额全量：gas 燃烧+罚没+历史销毁) 是**两个不同口径**：
	// 本字段只计 NV29「铸造即烧」的部分（f02 的 TotalBurnMinted），不是 f099 全量销毁。
	RewardStreamBurnMintedTotal decimal.Decimal `json:"reward_stream_burn_minted_total"`
	NV29Epoch                   int64           `json:"nv29_epoch"`          // NV29 激活高度；<=0/未排期 表示本网未激活
	GasIn32G                    decimal.Decimal `json:"gas_in_32g"`          // 32GiB扇区Gas消耗，单位Fil/T
	AddPowerIn32G               decimal.Decimal `json:"add_power_in_32g"`    // 32GiB扇区新增算力成本，单位Fil/T
	GasIn64G                    decimal.Decimal `json:"gas_in_64g"`          // 64GiB扇区Gas消耗，单位Fil/T
	AddPowerIn64G               decimal.Decimal `json:"add_power_in_64g"`    // 64GiB扇区新增算力成本，单位Fil/T
	WinCountReward              decimal.Decimal `json:"win_count_reward"`    // 每赢票奖励，单位Fil
	AvgBlockCount               decimal.Decimal `json:"avg_block_count"`     // 平均每高度区块数量
	AvgMessageCount             float64         `json:"avg_message_count"`   // 平均每高度消息数
	ActiveMiners                int64           `json:"active_miners"`       // 活跃节点数
	Burnt                       decimal.Decimal `json:"burnt"`               // 销毁量：f099(BurntFundsActor) **账户余额全量**（gas 燃烧+罚没+历史销毁）
	CirculatingPercent          decimal.Decimal `json:"circulating_percent"` // 流通率
	Sum                         decimal.Decimal `json:"sum"`
	ContractGas                 decimal.Decimal `json:"contract_gas"`
	Others                      decimal.Decimal `json:"others"`
}
