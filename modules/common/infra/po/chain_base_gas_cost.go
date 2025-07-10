package po

import "github.com/shopspring/decimal"

type BaseGasCostPo struct {
	Epoch         int64
	BaseGas       decimal.Decimal
	SectorGas32   decimal.Decimal
	SectorGas64   decimal.Decimal
	Messages      int64
	AccMessages   int64
	AvgGasLimit32 decimal.Decimal
	AvgGasLimit64 decimal.Decimal
	SectorFee32   decimal.Decimal
	SectorFee64   decimal.Decimal
}

func (BaseGasCostPo) TableName() string {
	return "chain.base_gas_costs"
}
