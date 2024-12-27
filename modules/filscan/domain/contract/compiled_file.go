package contract

type CompiledFile struct {
	ContractName     string
	ByteCode         string
	DeployedByteCode string
	Arguments        string
	ABI              string
	ContractFile     SourceFile
	IsMainContract   bool
}
