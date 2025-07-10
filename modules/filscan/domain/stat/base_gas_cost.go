package stat

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type BaseGasCost struct {
	Epoch         chain.Epoch
	BaseGas       chain.AttoFil
	SectorGas32   chain.AttoFil
	SectorGas64   chain.AttoFil
	Messages      int64
	AccMessages   int64
	AvgGasLimit32 decimal.Decimal
	AvgGasLimit64 decimal.Decimal
	SectorFee32   decimal.Decimal
	SectorFee64   decimal.Decimal
}
