package po

type DexInfo struct {
	ContractId string
	DexName    string
	DexUrl     string
	IconUrl    string
}

func (DexInfo) TableName() string {
	return "fevm.dex_info"
}
