package po

type ContractIcons struct {
	ContractId string `gorm:"column:contract_id"`
	Url        string `gorm:"column:url"`
}

func (ContractIcons) TableName() string {
	return "fevm.contract_icon"
}
