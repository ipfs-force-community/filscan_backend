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
	StatisticRewardStreams
	StatisticRewardStreamLedger
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

// StatisticRewardStreams 「区块奖励流向」：按窗口展示 NV29/FIP-0118 之后
// 区块奖励被拆成的矿工实收 / 服务流 / 销毁三股（数据＝ f02 计数器的相邻差分，不做权重建模）。
type StatisticRewardStreams interface {
	RewardStreams(ctx context.Context, req RewardStreamsRequest) (resp *RewardStreamsResponse, err error)
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
	Nv29Epoch int64          `json:"nv29_epoch"` // 本网 NV29 激活高度；未排期＝0（主网当前为 0）
	Items     []*DCTrendItem `json:"items"`      // 算力倍数结构走势图
}

type DCTrendItem struct {
	Epoch           int64           `json:"epoch"`
	BlockTime       int64           `json:"block_time"`
	Raw             decimal.Decimal `json:"raw"`               // 链上真值：原始算力
	QualityAdjPower decimal.Decimal `json:"quality_adj_power"` // 链上真值：有效算力
	// 算力倍数结构（NV29/FIP-0118 方案 A）：口径唯一实现在 chain.QualityTierSplit，
	// 前端只渲染、不再自行按 (qa−raw)/9 重算，避免两处口径分叉。
	FullMultiplierPower decimal.Decimal `json:"full_multiplier_power"` // 满倍率算力（10× 档等效原始字节）
	PendingUpgradePower decimal.Decimal `json:"pending_upgrade_power"` // 可升级算力（未达满倍率的等效原始字节，SP 可主动升到 10×）
}

// -----------------------区块奖励流向（NV29/FIP-0118）-----------------------//

type RewardStreamsRequest struct {
	Interval string `json:"interval"` // 时间间隔：24h / 7d / 30d
}

type RewardStreamsResponse struct {
	Nv29Epoch int64               `json:"nv29_epoch"` // 本网 NV29 激活高度；未排期＝0（主网当前为 0）
	Items     []*RewardStreamItem `json:"items"`      // 区块奖励流向列表
}

// RewardStreamItem 是「区块奖励流向」的一个数据点：相邻两个整点快照的计数器差分之和。
// 三个分量与总和的单位都是 **attoFIL**（原始值，与统计页其它曲线一致；前端用 formatFil ÷1e18 显示），
// total = miner + service + burn。
type RewardStreamItem struct {
	BlockTime int64           `json:"block_time"` // 区块时间（Unix 秒）
	Epoch     int64           `json:"epoch"`
	Miner     decimal.Decimal `json:"miner"`   // 矿工实收（共识流）
	Service   decimal.Decimal `json:"service"` // 服务流（f02 内部记账，事后 Claim）
	Burn      decimal.Decimal `json:"burn"`    // 销毁
	Total     decimal.Decimal `json:"total"`   // 三者之和
}

// -----------------------区块奖励服务流账本（NV29/FIP-0118）-----------------------//

// StatisticRewardStreamLedger 「服务流账本（当前状态快照）」：网络级展示 f02 奖励 actor 的实况——
// 待提取总额、当期已提取合计、当前分账比例（链上日程的评估权重）、以及各受益地址的份额与待付。
//
// 与 RewardStreams 的口径区分（不要混用）：
//   - RewardStreams  = f02 计数器的**窗口差分**，展示窗口内「已发生」的三股实收（历史事实）；
//   - RewardStreamLedger = ledger 的**当前状态**，展示「当前评估权重（日程）」与「待提取欠款」。
//     演示口径可能不同：权重按链上日程线性插值，实收占比还受期间赢票/出块分布影响。
//
// 数据源为 londobell 节点侧新端点 POST /adapter/reward_stream_ledger（取数走 acl → 节点，禁止本仓手搓 CBOR）。
type StatisticRewardStreamLedger interface {
	RewardStreamLedger(ctx context.Context, req RewardStreamLedgerRequest) (resp *RewardStreamLedgerResponse, err error)
}

type RewardStreamLedgerRequest struct {
}

type RewardStreamLedgerResponse struct {
	Epoch         int64                    `json:"epoch"`          // 账本读取高度
	Nv29          bool                     `json:"nv29"`           // 本网是否已激活 NV29；false 时金额字段为 "0"、recipients 为空
	PendingClaim  decimal.Decimal          `json:"pending_claim"`  // 待提取总额 = ledger.Liability()（attoFIL）
	ClaimedPeriod decimal.Decimal          `json:"claimed_period"` // 当期已提取合计（attoFIL）
	CurrentSplit  *RewardStreamSplit       `json:"current_split"`  // 当前评估权重百分比（链上日程，非实测占比）
	Recipients    []*RewardStreamRecipient `json:"recipients"`     // 各受益地址的份额%与待付
	// ClaimedSinceEpoch 「累计已收」口径的起始高度 = 归集表全表 MIN(first_epoch)：即这些累计值自哪个高度起有效
	// （= 采集件首次观测到任一受益方的高度）。0 表示**无数据 / 未知**（归集表本周期尚无行，或查询降级），
	// 与「累计恰为 0」区分：有数据时本字段为 MIN(first_epoch) 的正值（见 biz attachClaimedTotals）。
	ClaimedSinceEpoch int64 `json:"claimed_since_epoch"`
}

// RewardStreamSplit 当前分账比例（链上日程的评估权重百分比，一位小数）：
// miner = 隐式流（矿工）权重；service = 各显式流权重之和；burn = Denom − Σ权重。
type RewardStreamSplit struct {
	Miner   string `json:"miner"`
	Service string `json:"service"`
	Burn    string `json:"burn"`
}

// RewardStreamRecipient 单个受益地址：share_pct 是其在份额表中的占比（相对 Denom，两位小数，
// 与链上 RecipientShare.Share 同口径，不按 stream 权重折算）；pending_claim 是该地址可提取的欠款；
// claimed_period 是该地址本期已提取的金额（供前端「服务受益方排行」展示「已付」列）。
//
// removed_stream 标记「已移除奖励流（tombstone）的遗留欠款收款人」：该受益方**只**出现在已移除流的
// 遗留欠款（tombstone）里、当前份额表里没有它 ⇒ share_pct 显示 0.00%，pending_claim 是此前欠下、
// 链上仍挂在该地址名下、随时可 Claim 的结转余额。供前端在这类行上加「已移除流」标记与悬停说明。
// **不得**用 share_pct == 0 的启发式代替本字段：份额恰好为 0.00% 的活跃流会被误标。
type RewardStreamRecipient struct {
	Address      string          `json:"address"`       // 受益地址
	SharePct     string          `json:"share_pct"`     // 份额占比%（相对 Denom），两位小数
	PendingClaim decimal.Decimal `json:"pending_claim"` // 待付总额（attoFIL）＝ pending_claim_current + pending_claim_carried
	// PendingClaimCurrent：**当期**应收 ＝ 本期应计（accrued×share/denom）− 本期已提（claimed_period），不小于 0。
	// 链上依据：reward State.Accrued 注释为「current-period accrual」，ClaimedPeriod 为「current-period withdrawals」。
	PendingClaimCurrent decimal.Decimal `json:"pending_claim_current"`
	// PendingClaimCarried：**跨周期**应收 ＝ 此前各期已结算但未提取的结转（链上 Payable，
	// 注释为「settled but unclaimed amounts from prior periods」；已移除流的 tombstone 结转也计入）。
	// 分列口径（显示约定，保证两列之和恒等于 pending_claim）：本期提取先冲抵本期应计，超出部分再冲抵跨周期结转。
	PendingClaimCarried decimal.Decimal `json:"pending_claim_carried"`
	ClaimedPeriod       decimal.Decimal `json:"claimed_period"` // 该地址本期已提取金额（attoFIL 十进制字符串）
	RemovedStream       bool            `json:"removed_stream"` // 只出现在已移除流的遗留欠款里且当前份额为 0 ⇒ 遗留欠款收款人（true）
	// ZeroShare：该地址仍在**当前活跃的**服务流份额表里，但份额就是 0（该流本轮没给它分配权重）——
	// 与 RemovedStream 互斥（tombstone 命中优先）。true ⇒ 它没有新的应得，金额同样是此前结转、仍可提取的欠款。
	// 用例（2026-10-09 cali 实测）：地址移除 ≠ 流被删，链上可能留下「流还在、份额 0」的收款人。
	ZeroShare bool `json:"zero_share"`
	// ClaimedTotal：**累计已收**（attoFIL，跨周期求和）= 该地址在归集表 chain.reward_stream_recipient_period
	// 所有周期的 claimed_in_period 之和（含离场行；只增不减、跨周期保留）。与 ClaimedPeriod（**当期**已收，
	// 链上到下一周期归零）口径不同——「累计」≠「当期」。
	// **nil 表示未知**：归集表本周期尚无任何行（采集件未上生产 / 本周期内还没归集）或查询降级时，
	// 所有行都为 nil（JSON null），前端应显示「—」而非 0。
	// 非 nil 时：该地址在归集表无任何周期行 ⇒ 指向 0（确实从未提取，与「未知」区分）。
	ClaimedTotal *decimal.Decimal `json:"claimed_total"`
	// Departed：该受益方**本周期内已离场**——本周期归集表 ∪ 快照表里出现过，但当前链上状态（活跃份额表 + 已移除流
	// 遗留欠款）里已查不到（份额归零且欠款提完后，链上会彻底消失）。数据源为归集表（可靠、只增不减）
	// chain.reward_stream_recipient_period 与快照表 chain.reward_stream_recipient_epoch 的并集；
	// 归集表只增不减，故即便快照表被框架 HistoryClear 修剪，离场仍可识别。两来源任一查询失败时按空处理、
	// 接口照常返回。口径与补齐规则见 biz 层 buildRewardStreamRecipients 注释「本周期离场者」一节。
	Departed bool `json:"departed"`
	// LeftEpoch：离场高度 —— 两表都可用时取较大者（最近一次被看到的高度）：快照表本周期最大 epoch
	// 与归集表该地址 last_epoch 的较大值。仅 Departed=true 时有意义，其余为 0。
	LeftEpoch int64 `json:"left_epoch"`
	// LastSharePct：离开前的份额占比%（**口径同 SharePct**，相对 Denom、两位小数，如 "73.75"）——
	// 优先取快照表本周期内 epoch 最大那一行的 Share；快照没有该地址时退回归集表该地址的 last_share 字段
	// （同一 sharePercent 口径）。仅 Departed=true 时有意义，其余为 ""。
	LastSharePct string `json:"last_share_pct"`
}

// ClaimedPeriod 口径说明：来源是链上 f02 奖励流账本的 recipient 级 ClaimedPeriod（**当前期**口径，非累计）；
// tombstone（已移除流）的收款人没有该字段，取 0。
