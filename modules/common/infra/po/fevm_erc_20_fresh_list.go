package po

type FEvmErc20FreshList struct {
	ContractId string
}

func (FEvmErc20FreshList) TableName() string {
	return "fevm.erc20_fresh_list"
}
