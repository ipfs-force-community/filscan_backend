package bo

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type MinerInfo struct {
	Epoch                  int64
	Miner                  string            // 账户ID
	Owner                  string            // Owner地址
	Worker                 string            // Worker地址
	Controllers            types.StringArray // Controllers地址列表
	Balance                decimal.Decimal   // 账户总余额
	AvailableBalance       decimal.Decimal   // 可用余额
	InitialPledge          decimal.Decimal   // 扇区质押(初始抵押)
	PreCommitDeposits      decimal.Decimal   // 预存款
	LockedBalance          decimal.Decimal   // 锁仓奖励(挖矿锁定)
	QualityAdjPower        decimal.Decimal   // 有效算力
	QualityAdjPowerRank    int64             // 有效算力排行
	QualityAdjPowerPercent decimal.Decimal   // 有效算力占比
	RawBytePower           decimal.Decimal   // 原值算力
	AccBlockCount          int64             // 总出块数
	AccReward              decimal.Decimal   // 总出块奖励
	AccWinCount            int64             // 总赢票数
	SectorSize             int64             // 扇区大小
	SectorCount            int64             // 扇区总数
	LiveSectorCount        int64             // 有效扇区
	FaultSectorCount       int64             // 错误扇区
	RecoverSectorCount     int64             // 恢复扇区
	ActiveSectorCount      int64             // 活跃扇区
	TerminateSectorCount   int64             // 终止扇区
}

type AccGasFee struct {
	Miner     string
	PreAgg    decimal.Decimal
	ProveAgg  decimal.Decimal
	SectorGas decimal.Decimal
	SealGas   decimal.Decimal // SealGas = PreAgg + ProveAgg + SectorGas
	WdPostGas decimal.Decimal
}

type AccWinCount struct {
	Miner    string
	WinCount int64
	// GasReward 区间内 gas 奖励之和（attoFIL，对齐聚合器 wincount 的 TotalGasReward）。
	//
	// 用**指针**表达 NULL：chain.miner_win_counts.gas_reward 在 migration/36 之前的行、
	// 以及尚未回填的行上是 NULL，而 sum() 遇到全 NULL 得 NULL。
	// NULL ⇒「本区间还没回填」⇒ 调用方（agg_pg_reward.go）必须回落聚合器，
	// 不能当 0 用（0 是聚合器的合法取值）。
	GasReward *decimal.Decimal
	// TotalRows / GasRewardRows 去重后的行数 / 其中有 gas_reward 的行数。
	// GasRewardRows < TotalRows ⇒ 区间内还有未回填行 ⇒ 汇总值不完整。
	TotalRows     int64
	GasRewardRows int64
}

type AccReward struct {
	Miner      string
	Reward     decimal.Decimal
	BlockCount int64
}

type GasPerT struct {
	Epoch  int64
	Gas32G decimal.Decimal
	Gas64G decimal.Decimal
}

type MinerCount struct {
	SectorSize      int64
	QualityAdjPower decimal.Decimal
}
