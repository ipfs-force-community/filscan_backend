package po

import "time"

type FEvmContracts struct {
	ActorID         string
	ActorAddress    string
	ContractAddress string
	Arguments       string
	License         string
	Language        string
	Compiler        string
	Optimize        bool
	OptimizeRuns    int64
	Verified        bool
	CreatedAt       time.Time
}

func (c FEvmContracts) TableName() string {
	return "fevm.contracts"
}

type FEvmContractSols struct {
	ActorID        string
	FileName       string
	Source         string
	Size           int64
	ContractName   string
	ByteCode       string
	ABI            string
	CreatedAt      time.Time
	IsMainContract bool
}

func (c FEvmContractSols) TableName() string {
	return "fevm.contract_sols"
}
