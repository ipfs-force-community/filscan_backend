package po

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type MinerInfo struct {
	Epoch                  int64
	Miner                  string
	Owner                  string
	Worker                 string
	Controllers            types.StringArray
	RawBytePower           decimal.Decimal
	QualityAdjPower        decimal.Decimal
	Balance                decimal.Decimal
	AvailableBalance       decimal.Decimal
	VestingFunds           decimal.Decimal
	FeeDebt                decimal.Decimal
	SectorSize             int64
	SectorCount            int64
	FaultSectorCount       int64
	ActiveSectorCount      int64
	LiveSectorCount        int64
	RecoverSectorCount     int64
	TerminateSectorCount   int64
	PreCommitSectorCount   int64
	InitialPledge          decimal.Decimal
	PreCommitDeposits      decimal.Decimal
	QualityAdjPowerRank    int64
	QualityAdjPowerPercent decimal.Decimal
	Ips                    types.StringArray
}

func (MinerInfo) TableName() string {
	return "chain.miner_infos"
}
