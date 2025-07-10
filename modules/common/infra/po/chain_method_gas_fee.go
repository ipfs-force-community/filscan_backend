package po

import "github.com/shopspring/decimal"

type MethodGasFee struct {
	Epoch      int64
	Method     string
	Count      int64
	GasPremium decimal.Decimal
	GasLimit   decimal.Decimal
	GasCost    decimal.Decimal
	GasFee     decimal.Decimal
}

func (MethodGasFee) TableName() string {
	return "chain.method_gas_fees"
}
