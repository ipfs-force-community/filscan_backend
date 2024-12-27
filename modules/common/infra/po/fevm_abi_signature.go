package po

type FEvmABISignature struct {
	Type string
	Name string
	Id   string
	Raw  string
}

func (FEvmABISignature) TableName() string {
	return "fevm.abi_signatures"
}
