package po

import (
	"github.com/shopspring/decimal"
)

type EvmTransfer struct {
	Epoch        int64
	MessageCid   string
	ActorID      string
	ActorAddress string
	UserAddress  string
	Balance      decimal.Decimal
	GasCost      decimal.Decimal
	Value        decimal.Decimal
	ExitCode     *int
	MethodName   string
}

func (c EvmTransfer) TableName() string {
	return "fevm.evm_transfers"
}

type EvmTransferStat struct {
	Epoch            int64
	ActorID          string
	Interval         string
	AccTransferCount int64
	AccUserCount     int64
	AccGasCost       decimal.Decimal
	ActorBalance     decimal.Decimal
	ActorAddress     string
	ContractAddress  string
	ContractName     string
}

func (c EvmTransferStat) TableName() string {
	return "fevm.evm_transfer_stats"
}
