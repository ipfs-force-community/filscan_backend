package bo

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type EvmTransfers struct {
	ActorID         string
	ActorAddress    string
	ContractAddress string
	ContractName    string
	TransferCount   int64
	UserCount       int64
	GasCost         decimal.Decimal
	ActorBalance    decimal.Decimal
}

type EVMTransferStats struct {
	ActorID          string
	ActorAddress     string
	ContractName     string
	AccTransferCount int64
	AccUserCount     int64
	AccGasCost       decimal.Decimal
	ActorBalance     decimal.Decimal
}

type EVMTransferStatsWithName struct {
	ActorID          string
	ContractName     string
	AccTransferCount int64
	AccUserCount     int64
	AccGasCost       decimal.Decimal
}

type EVMTransactionStats struct {
	Epoch               chain.Epoch
	ActorID             string
	ActorAddress        string
	ContractName        string
	AccTransactionCount int64
	AccInternalTxCount  int64
	AccUserCount        int64
	AccGasCost          decimal.Decimal
	ActorBalance        decimal.Decimal
}

type ContractTotalBalance struct {
	Epoch   chain.Epoch
	Balance decimal.Decimal
}

type ContractCnt struct {
	Epoch chain.Epoch
	Cnts  int64
}
