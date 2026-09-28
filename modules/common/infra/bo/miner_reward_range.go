package bo

import "github.com/shopspring/decimal"

// MinerEpochReward 逐 epoch 逐矿工的出块奖励（PG 派生表 chain.miner_rewards 的口径）。
//
// 字段与聚合器 miner_blockreward / miners_blockreward 一一对应：
//
//	Reward     ← TotalBlockReward（单位 attoFIL，decimal，原样透传，不做单位换算）
//	BlockCount ← BlockCount（该 epoch 该矿工的出块数）
//	Epoch/Miner← 聚合结果的分组键（miner_blockreward 按 Epoch 分组、miners_blockreward 按 Epoch+Miner 分组）
//
// 与工程内既有写法保持一致：金额用 decimal.Decimal 承接到此层，转换/断言发生在 biz 层。
type MinerEpochReward struct {
	Epoch      int64
	Miner      string
	Reward     decimal.Decimal
	BlockCount int64
}
