package bo

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type OwnerInfo struct {
	Epoch                  int64
	Owner                  string
	Miners                 types.StringArray
	Balance                decimal.Decimal // 账户总余额
	AvailableBalance       decimal.Decimal // 可用余额
	InitialPledge          decimal.Decimal // 扇区质押(初始抵押)
	PreCommitDeposits      decimal.Decimal // 预存款
	LockedBalance          decimal.Decimal // 锁仓奖励(挖矿锁定)
	QualityAdjPower        decimal.Decimal // 有效算力
	QualityAdjPowerRank    int64           // 有效算力排名
	QualityAdjPowerPercent decimal.Decimal // 有效算力占比
	RawBytePower           decimal.Decimal // 原值算力
	AccBlockCount          int64           // 总出块数
	AccReward              decimal.Decimal // 总出块奖励
	AccWinCount            int64           // 总赢票数
	SectorSize             int64           // 扇区大小
	SectorCount            int64           // 扇区总数
	LiveSectorCount        int64           // 有效扇区
	FaultSectorCount       int64           // 错误扇区
	RecoverSectorCount     int64           // 恢复扇区
	ActiveSectorCount      int64           // 活跃扇区
	TerminateSectorCount   int64           // 终止扇区
}
