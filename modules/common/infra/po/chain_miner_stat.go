package po

import "github.com/shopspring/decimal"

type MinerStat struct {
	Epoch                 int64
	Miner                 string
	Interval              string
	PrevEpochRef          int64
	RawBytePowerChange    decimal.Decimal
	QualityAdjPowerChange decimal.Decimal
	SectorCountChange     int64
	InitialPledgeChange   decimal.Decimal
	AccReward             decimal.Decimal
	AccRewardPercent      decimal.Decimal
	AccBlockCount         int64
	AccBlockCountPercent  decimal.Decimal
	AccWinCount           int64
	AccSealGas            decimal.Decimal
	AccWdPostGas          decimal.Decimal
	RewardPowerRatio      decimal.Decimal
	WiningRate            decimal.Decimal
	LuckRate              decimal.Decimal
	sectorSize            int64
	qualityAdjPower       decimal.Decimal
}

func (m *MinerStat) QualityAdjPower() decimal.Decimal {
	return m.qualityAdjPower
}

func (m *MinerStat) SetQualityAdjPower(qualityAdjPower decimal.Decimal) {
	m.qualityAdjPower = qualityAdjPower
}

func (m *MinerStat) SectorSize() int64 {
	return m.sectorSize
}

func (m *MinerStat) SetSectorSize(sectorSize int64) {
	m.sectorSize = sectorSize
}

func (m *MinerStat) TableName() string {
	return "chain.miner_stats"
}
