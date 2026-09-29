package po

import (
	"github.com/shopspring/decimal"
)

type MinerWinCount struct {
	Epoch    int64
	Miner    string
	WinCount int64
	// GasReward 该 (epoch, miner) 的 gas 奖励（attoFIL）。
	//
	// 来源：聚合器端点 /aggregators/wincount 的 TotalGasReward —— 也就是同步器**已经在调**的
	// 那一次 ctx.Agg().WinCount(...) 响应里的同一个字段（管线见
	// londobell-aggregators/pool-monitor/wincount_zl.js：Message.Detail.Params.GasReward，
	// 即 RewardActor.AwardBlockReward 隐式消息的 params 之一）。落库不新增任何数据源/请求。
	//
	// NULL 与 0 语义不同：migration/36 之前的行（以及尚未回填的行）是 NULL，而 0 是聚合器的
	// **合法取值**（线上实测 epoch 6330000 有 10 个矿工 TotalGasReward 就是 "0"）。
	// 读路径（dal_biz_miner_reward_range.go / agg_pg_reward.go）用 NULL 判「未回填」并回落聚合器，
	// 不允许把 NULL 当成 0 用 —— 该字段被 acl_block_chain.GetBlockDetails 用来算
	// TxFeeReward / MinedReward（assembler_block_chain_info.go:42-58）。
	GasReward decimal.Decimal
}

func (MinerWinCount) TableName() string {
	return "chain.miner_win_counts"
}
