package po

import "github.com/shopspring/decimal"

// RewardStreamRecipientPeriod 是 f02 奖励流（NV29 / FIP-0118）受益方的**按周期归集**：
// 每个（受益地址，周期）一行，纪录该周期内链上 claimed_period 的最大观测值 —— 用来支撑
// 产品页面的「累计已收」。链上 ClaimedPeriod 到下一周期就归零，累计值链上并不存在，
// 只能跨周期累加（上层按 address 取全部周期行再 SUM）。
//
// 与 chain.reward_stream_recipient_epoch（按高度快照）的关键差别：
//   - 快照表可被框架 HistoryClear 按高度**修剪**，只服务当前周期；
//   - 本表**只增不减、跨周期保留**，是累计已收的唯一来源（HistoryClear 不清理本表）。
//
// 落库规则见同步器计算器 modules/syncer/calculator/calc-reward-stream-recipient-task：
//   - 每个高度对每条**显式流**（Implicit=false）的每个受益方，把链上 claimed_period 累加到
//     (Address, PeriodStartEpoch)；同一地址出现在多条显式流里时按地址累加；
//   - ledger.tombstones 的遗留受益方 claimed_period 记 0，**不进本表**；
//   - 同一 (address, period) 跨高度合并写：ClaimedInPeriod 取 GREATEST、
//     FirstEpoch 取 LEAST、LastEpoch 取 GREATEST（幂等，重跑不降值）；
//     LastShare 取「观测高度更大」的那一行的值（同高度重跑取新值，绝不用陈旧份额盖掉更新的）。
//
// 数值列一律用 decimal（attoFIL / 1e18 定点），**不得**用 float（attoFIL 远超 float64 有效位数）。
// DDL 见 migration/38.reward_stream_recipient_period.sql。
type RewardStreamRecipientPeriod struct {
	Address          string          // 受益方地址（robust 原文）
	PeriodStartEpoch int64           // 周期起点高度（周期标识）
	ClaimedInPeriod  decimal.Decimal // 本周期已提（attoFIL）= 该周期内链上 claimed_period 最大观测值
	// LastShare 该周期内最近一次观测到的份额之和（Denom=1e18 定点；按地址跨显式流累加）。
	// 供展示层给「本周期离场者」补 last_share_pct（离场后链上查不到其当前份额，只有归集表留痕）。
	// 口径与快照表 Share 一致（同一 sharePercent 折算）。合并写按 LastEpoch 取更新的那一行的值。
	LastShare  decimal.Decimal
	FirstEpoch int64 // 本周期内首次观测到该受益方的高度
	LastEpoch  int64 // 本周期内最近一次观测到该受益方的高度
}

func (RewardStreamRecipientPeriod) TableName() string {
	return "chain.reward_stream_recipient_period"
}
