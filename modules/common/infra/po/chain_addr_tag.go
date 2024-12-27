package po

type AddressTag struct {
	ID      int64
	Address string
	Tag     string
}

func (AddressTag) TableName() string {
	return "chain.addr_tags"
}
