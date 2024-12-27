package filscan

import (
	"context"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type StatisticAPI interface {
	StatisticBaseLineTrend
	StatisticBaseFeeTrend
	StatisticActiveMinerTrend
	StatisticBlockRewardTrend
	StatisticMessageCountTrend
	StatisticGasDataTrend
	StatisticDCTrend
	StatisticContractTrend
	FilCompose(ctx context.Context, req FilComposeRequest) (resp FilComposeResponse, err error)
	PeerMap(ctx context.Context, req PeerMapRequest) (resp PeerMapResponse, err error)
}

type StatisticBaseLineTrend interface {
	BaseLineTrend(ctx context.Context, req BaseLineTrendRequest) (resp *BaseLineTrendResponse, err error)
}

type StatisticContractTrend interface {
	ContractUsersTrend(ctx context.Context, req ContractUsersTrendRequest) (resp *ContractUsersTrendResponse, err error)
	ContractCntTrend(ctx context.Context, req ContractCntTrendRequest) (resp *ContractCntTrendResponse, err error)
	ContractTxsTrend(ctx context.Context, req ContractTxsTrendRequest) (resp *ContractTxsTrendResponse, err error)
	ContractBalanceTrend(ctx context.Context, req ContractBalanceTrendRequest) (resp *ContractBalanceTrendResponse, err error)
}

type StatisticBaseFeeTrend interface {
	BaseFeeTrend(ctx context.Context, req BaseFeeTrendRequest) (resp *BaseFeeTrendResponse, err error)
}

type StatisticBlockRewardTrend interface {
	BlockRewardTrend(ctx context.Context, req BlockRewardTrendRequest) (resp *BlockRewardTrendResponse, err error)
}

type StatisticActiveMinerTrend interface {
	ActiveMinerTrend(ctx context.Context, req ActiveMinerTrendRequest) (resp *ActiveMinerTrendResponse, err error)
}

type StatisticMessageCountTrend interface {
	MessageCountTrend(ctx context.Context, req MessageCountTrendRequest) (resp *MessageCountTrendResponse, err error)
}

type StatisticGasDataTrend interface {
	GasDataTrend(ctx context.Context, req GasDataTrendRequest) (resp *GasDataTrendResponse, err error) // 24小时 gas 数据
}

type StatisticDCTrend interface {
	DCTrend(ctx context.Context, req DCTrendRequest) (resp DCTrendResponse, err error)
}

// -----------------------统计页接口参数结构-----------------------

type BaseLineTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type BaseLineTrendResponse struct {
	Epoch     int64
	BlockTime int64
	List      []*BaseLineTrend `json:"list"` // 基线走势列表
}

type ContractTxsTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type ContractTxsTrendResponse struct {
	Epoch     int64               `json:"epoch"`
	BlockTime int64               `json:"block_time"`
	Items     []*ContractTxsTrend `json:"items"` // 合约交易数走势列表
}

type ContractBalanceTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type ContractBalanceTrendResponse struct {
	Epoch     int64                   `json:"epoch"`
	BlockTime int64                   `json:"block_time"`
	Items     []*ContractBalanceTrend `json:"items"` // 合约余额走势列表
}

type ContractUsersTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type ContractUsersTrendResponse struct {
	Epoch     int64                 `json:"epoch"`
	BlockTime int64                 `json:"block_time"`
	Items     []*ContractUsersTrend `json:"items"` // 合约交易地址走势列表
}

type ContractCntTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type ContractCntTrendResponse struct {
	Epoch     int64               `json:"epoch"`
	BlockTime int64               `json:"block_time"`
	Items     []*ContractCntTrend `json:"items"` // 合约交易地址走势列表
}

type BaseFeeTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type BaseFeeTrendResponse struct {
	Epoch     int64
	BlockTime int64
	List      []BaseFeeTrend `json:"list"` // 基础手续费走势列表
}

type Gas24HTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type GasDataTrendRequest struct {
}

type GasDataTrendResponse struct {
	Epoch     int64
	BlockTime int64
	Items     []*GasDataTrend `json:"items"` // Gas数据趋势列表
}

type FilComposeRequest struct {
}

type FilComposeResponse struct {
	FilCompose FilCompose `json:"fil_compose"` // Fil使用途径图表统计
}

type BlockRewardTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type BlockRewardTrendResponse struct {
	Items     []*BlockRewardTrend `json:"items"` // 区块奖励列表
	Epoch     int64               `json:"epoch"`
	BlockTime string              `json:"block_time"`
}

type ActiveMinerTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type ActiveMinerTrendResponse struct {
	Epoch     int64               `json:"epoch"`
	BlockTime int64               `json:"block_time"`
	Items     []*ActiveMinerTrend `json:"items"` // 活跃节点走势列表
}

type MessageCountTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔
}

type MessageCountTrendResponse struct {
	Items     []*MessageCountTrend `json:"items"` // 消息数走势列表
	Epoch     int64                `json:"epoch"`
	BlockTime string               `json:"block_time"`
}

type PeerMapRequest struct {
}

type PeerMapResponse struct {
	PeerMapList []PeerMap `json:"peer_map_list"` // 节点地图列表
}

// -----------------------统计页基础数据结构-----------------------

type BaseLineTrend struct {
	TotalQualityAdjPower  decimal.Decimal `json:"total_quality_adj_power"`  // 全网有效算力
	TotalRawBytePower     decimal.Decimal `json:"total_raw_byte_power"`     // 全网原值算力
	BaseLinePower         decimal.Decimal `json:"base_line_power"`          // 基线算力
	ChangeQualityAdjPower decimal.Decimal `json:"change_quality_adj_power"` // 环比变化有效算力
	Timestamp             int64           `json:"timestamp"`                // 时间戳
	Epoch                 chain.Epoch     `json:"-"`
	PowerIncrease         decimal.Decimal `json:"power_increase"`
	PowerDecrease         decimal.Decimal `json:"power_decrease"`
}

type BaseFeeTrend struct {
	BaseFee   decimal.Decimal `json:"base_fee"`   // 当前基础手续费
	GasIn32G  decimal.Decimal `json:"gas_in_32g"` // 32GiB扇区Gas消耗，单位Fil/T
	GasIn64G  decimal.Decimal `json:"gas_in_64g"` // 64GiB扇区Gas消耗，单位Fil/T
	Timestamp string          `json:"timestamp"`  // 区块时间
}

type GasDataTrend struct {
	MethodName        string          `json:"method_name"`         // 消息类型
	AvgGasPremium     decimal.Decimal `json:"avg_gas_premium"`     // 平均小费费率
	AvgGasLimit       decimal.Decimal `json:"avg_gas_limit"`       // 平均Gas限额
	AvgGasUsed        decimal.Decimal `json:"avg_gas_used"`        // 平均Gas消耗
	AvgGasFee         decimal.Decimal `json:"avg_gas_fee"`         // 平均手续费
	SumGasFee         decimal.Decimal `json:"sum_gas_fee"`         // 合计手续费
	GasFeeRatio       decimal.Decimal `json:"gas_fee_ratio"`       // 手续费占比
	MessageCount      int64           `json:"message_count"`       // 消息数
	MessageCountRatio decimal.Decimal `json:"message_count_ratio"` // 消息数占比
}

type FilCompose struct {
	Mined             decimal.Decimal `json:"mined"`              // 已提供存储者奖励的Fil
	RemainingMined    decimal.Decimal `json:"remaining_mined"`    // 剩余存储者奖励的Fil
	Vested            decimal.Decimal `json:"vested"`             // 已释放锁仓奖励的Fil
	RemainingVested   decimal.Decimal `json:"remaining_vested"`   // 剩余锁仓奖励的Fil
	ReserveDisbursed  decimal.Decimal `json:"reserve_disbursed"`  // 已分配保留部分的Fil
	RemainingReserved decimal.Decimal `json:"remaining_reserved"` // 剩余保留部分的Fil
	Locked            decimal.Decimal `json:"locked"`             // 扇区抵押的Fil
	Burnt             decimal.Decimal `json:"burnt"`              // 已销毁的Fil
	Circulating       decimal.Decimal `json:"circulating"`        // 可交易流通的Fil
	TotalReleased     decimal.Decimal `json:"total_released"`     // 全部已释放的Fil
}

type BlockRewardTrend struct {
	BlockTime         int64           `json:"block_time"`           // 区块时间
	AccBlockRewards   decimal.Decimal `json:"acc_block_rewards"`    // 累计区块奖励
	BlockRewardPerTib decimal.Decimal `json:"block_reward_per_tib"` // 每Tib算力产出效率
}

type ActiveMinerTrend struct {
	BlockTime        int64 `json:"block_time"`         // 区块时间
	ActiveMinerCount int64 `json:"active_miner_count"` // 活跃节点数
}

type ContractTxsTrend struct {
	BlockTime   int64 `json:"block_time"`   // 区块时间
	ContractTxs int64 `json:"contract_txs"` // 合约交易数
}

type ContractBalanceTrend struct {
	BlockTime            int64           `json:"block_time"`             // 区块时间
	ContractTotalBalance decimal.Decimal `json:"contract_total_balance"` // 合约总的余额
}

type ContractUsersTrend struct {
	BlockTime     int64 `json:"block_time"`     // 区块时间
	ContractUsers int64 `json:"contract_users"` // 合约交易地址数
}

type ContractCntTrend struct {
	BlockTime    int64 `json:"block_time"`      // 区块时间
	ContractCnts int64 `json:"contract_counts"` // 合约部署地址数
}

type MessageCountTrend struct {
	BlockTime       string `json:"block_time"`        // 区块时间
	MessageCount    int64  `json:"message_count"`     // 消息数量
	AllMessageCount int64  `json:"all_message_count"` // 总消息数量
}

type PeerMap struct {
	Latitude   string `json:"latitude"`    // 纬度坐标
	Longitude  string `json:"longitude"`   // 经度坐标
	LocationCN string `json:"location_cn"` // 中文位置名称
	LocationEN string `json:"location_en"` // 英文位置名称
	IP         string `json:"ip"`          // ip地址
	MinerID    string `json:"miner_id"`    // 节点ID
}

type DCTrendRequest struct {
	Interval string `json:"interval"` // 时间间隔

}

type DCTrendResponse struct {
	Epoch     int64          `json:"epoch"`
	BlockTime int64          `json:"block_time"`
	Items     []*DCTrendItem `json:"items"` // DC占比走势图
}

type DCTrendItem struct {
	Epoch     int64           `json:"epoch"`
	BlockTime int64           `json:"block_time"`
	Dc        decimal.Decimal `json:"dc"`
	Cc        decimal.Decimal `json:"cc"`
}
