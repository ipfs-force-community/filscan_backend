package bo

import "github.com/shopspring/decimal"

type LuckQualityAdjPower struct {
	Epoch           int64
	QualityAdjPower decimal.Decimal
}

type LuckMinerTicket struct {
	Height    int64
	Miner     string
	WinCounts int64
}

type LuckNetTicket struct {
	Height    int64
	WinCounts int64
}
