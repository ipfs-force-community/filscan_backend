package bo

import "time"

type VerifiedContracts struct {
	ActorID         string
	ActorAddress    string
	ContractAddress string
	Arguments       string
	License         string
	Language        string
	Compiler        string
	Optimize        bool
	OptimizeRuns    int64
	ContractName    string
	ByteCode        string
	ABI             string
	CreatedAt       time.Time
}
