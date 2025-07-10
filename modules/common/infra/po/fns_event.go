package po

import "gitlab.forceup.in/fil-data-factory/filscan-backend/types"

type FNSEvent struct {
	Epoch      int64
	Cid        string
	LogIndex   int64
	Contract   string
	EventName  string
	Topics     types.StringArray
	Data       string
	Removed    bool
	MethodName string
}

func (FNSEvent) TableName() string {
	return "fns.events"
}
