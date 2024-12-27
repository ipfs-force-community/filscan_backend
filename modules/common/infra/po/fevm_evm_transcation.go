package po

import "github.com/shopspring/decimal"

type EvmTransaction struct {
	TraceID      string
	Epoch        int64
	MessageCid   string
	ActorID      string
	ActorAddress string
	UserAddress  string
	GasCost      decimal.Decimal
	IsBlock      bool
	Value        decimal.Decimal
	ExitCode     *int
	MethodName   string
}

func (e EvmTransaction) TableName() string {
	return "fevm.evm_transactions"
}

type EvmTransactionStat struct {
	Epoch               int64
	ActorID             string
	AccTransactionCount int64
	AccInternalTxCount  int64
	AccUserCount        int64
	AccGasCost          decimal.Decimal
	ActorAddress        string
	ContractAddress     string
	ContractName        string
}

func (e EvmTransactionStat) TableName() string {
	return "fevm.evm_transaction_stats"
}

type EvmTransactionUser struct {
	ActorID       string
	UserAddress   string
	LatestTxEpoch int64
}

func (e EvmTransactionUser) TableName() string {
	return "fevm.evm_transaction_users"
}
