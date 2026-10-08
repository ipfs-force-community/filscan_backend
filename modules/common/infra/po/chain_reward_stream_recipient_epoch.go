package po

import "github.com/shopspring/decimal"

// RewardStreamRecipientEpoch 是 f02 奖励流（NV29 / FIP-0118）受益方的**按高度快照**：
// 每个高度、每个受益地址一行，用来留痕「本周期内出现过的受益方」——
// 链上只保存当前状态，受益方被移出且欠款提完后会彻底消失，单看链头查不到。
//
// 落库规则见同步器计算器 modules/syncer/calculator/calc-reward-stream-recipient-task：
//   - 每条显式流（Implicit=false）的每个受益方：Share / Payable / ClaimedPeriod 取链上值；
//   - ledger.tombstones 里的遗留受益方：Share 记 0、Payable 取链上值、ClaimedPeriod 记 0、Tombstone=true；
//   - 同一地址出现在多条流里时按地址合并（唯一键 (epoch, address)）。
//
// 数值列一律用 decimal（attoFIL / 1e18 定点），**不得**用 float（attoFIL 远超 float64 有效位数）。
// DDL 见 migration/37.reward_stream_recipient_epoch.sql。
type RewardStreamRecipientEpoch struct {
	Epoch         int64           // 高度
	Address       string          // 受益方地址（robust 原文）
	Share         decimal.Decimal // 份额之和（Denom=1e18 定点；纯 tombstone 行为 0）
	Payable       decimal.Decimal // 跨周期结转欠款（attoFIL）
	ClaimedPeriod decimal.Decimal // 本期已提（attoFIL；tombstone 行记 0）
	Tombstone     bool            // 该地址在本高度出现在已移除流（ledger.tombstones）里
}

func (RewardStreamRecipientEpoch) TableName() string {
	return "chain.reward_stream_recipient_epoch"
}
