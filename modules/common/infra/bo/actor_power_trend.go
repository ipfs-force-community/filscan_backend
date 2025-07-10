package bo

import "github.com/shopspring/decimal"

type ActorPowerTrend struct {
	Epoch       int64
	AccountID   string          // 账户ID
	BlockTime   int64           // 区块时间
	Power       decimal.Decimal // 有效算力
	PowerChange decimal.Decimal // 有效算力增长24h
}
