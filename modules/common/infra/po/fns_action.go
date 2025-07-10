package po

const (
	FNSActionNew    = 1
	FNSActionUpdate = 2
	FNSActionDelete = 3
)

type FNSAction struct {
	Epoch    int64
	Name     string
	Provider string
	Action   int
}

func (FNSAction) TableName() string {
	return "fns.actions"
}
