package po

type FNSToken struct {
	Name           string
	Provider       string
	TokenId        string
	Node           string
	Registrant     string
	Controller     string
	ExpiredAt      int64
	FilAddress     string
	LastEventEpoch int64
}

func (FNSToken) TableName() string {
	return "fns.tokens"
}
