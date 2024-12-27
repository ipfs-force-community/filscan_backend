package po

type FNSTransfer struct {
	Epoch    int64
	Provider string
	Cid      string
	LogIndex int64
	Method   string
	From     string
	To       string
	TokenId  string
	Contract string
	Item     string
}

func (FNSTransfer) TableName() string {
	return "fns.transfers"
}
