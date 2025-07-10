package bo

import "github.com/shopspring/decimal"

type MinerRewardRank struct {
	Miner            string
	AccReward        decimal.Decimal
	AccRewardPercent decimal.Decimal
	AccBlockCount    int64
	QualityAdjPower  decimal.Decimal
	SectorSize       int64
	WiningRate       decimal.Decimal
}
