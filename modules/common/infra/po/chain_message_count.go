package po

type MessageCount struct {
	Epoch           int64
	Message         int64
	Block           int64
	AvgBlockMessage int64
}

func (MessageCount) TableName() string {
	return "chain.message_counts"
}
