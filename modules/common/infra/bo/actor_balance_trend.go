package bo

import "github.com/shopspring/decimal"

type ActorBalanceTrend struct {
	Epoch             int64
	AccountID         string           // 账户ID
	Balance           decimal.Decimal  // 账户总余额
	AvailableBalance  *decimal.Decimal // 可用余额
	InitialPledge     *decimal.Decimal // 扇区质押(初始抵押)
	PreCommitDeposits *decimal.Decimal // 预存款
	LockedBalance     *decimal.Decimal // 锁仓奖励(挖矿锁定)
}
