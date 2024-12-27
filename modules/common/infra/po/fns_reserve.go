package po

type FnsReserve struct {
	Address string
	Domain  *string
	Epoch   int64
}

func (FnsReserve) TableName() string {
	return "fns.reverses"
}
