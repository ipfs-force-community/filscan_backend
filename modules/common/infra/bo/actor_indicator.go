package bo

import "github.com/shopspring/decimal"

type ActorIndicator struct {
	Epoch                 int64
	Miner                 string
	Owner                 string
	SealPowerChange       decimal.Decimal
	QualityAdjPower       decimal.Decimal
	QualityAdjPowerChange decimal.Decimal
	AccSealGas            decimal.Decimal
	AccWdPostGas          decimal.Decimal
	AccWinCount           int64
	AccReward             decimal.Decimal
	AccBlockCount         int64
	SectorCountChange     int64
	InitialPledgeChange   decimal.Decimal
	LuckRate              decimal.Decimal
	//SectorRatio         decimal.Decimal // 扇区增速
	//SectorDeposits      decimal.Decimal // 扇区抵押
	//GasFee              decimal.Decimal // gas消耗
	//BlockCountIncrease  int64           // 出块增量
	//BlockRewardIncrease decimal.Decimal // 出块奖励
	//WinCount            int64           // 赢票数量
	//RewardsPerTB        decimal.Decimal // 效率(出块奖励/有效算力:FIL/TiB)
	//GasFeePerTB         decimal.Decimal // 单T消耗(gas消耗/扇区增量:FIL/TiB)
	//Lucky               decimal.Decimal // 幸运值
}

/**\
a.epoch,
       a.miner,
       a.sector_size * b.live_sector_change as seal_power_change,
       a.quality_adj_power_rank,
       b.quality_adj_power_change,
       b.acc_seal_gas,
       b.acc_wd_post_gas,
       b.acc_win_count,
       b.acc_reward,
       b.acc_block_count
*/
