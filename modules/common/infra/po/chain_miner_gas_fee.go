package po

import "github.com/shopspring/decimal"

type MinerGasFee struct {
	Epoch     int64
	Miner     string
	PreAgg    decimal.Decimal
	ProveAgg  decimal.Decimal
	SectorGas decimal.Decimal
	SealGas   decimal.Decimal // SealGas = PreAgg + ProveAgg + SectorGas
	WdPostGas decimal.Decimal
}

func (m MinerGasFee) TableName() string {
	return "chain.miner_gas_fees"
}
