package po

const (
	ActorActionNew    = 1
	ActorActionUpdate = 2
)

type ActorAction struct {
	Epoch   int64
	ActorId string
	Action  int
}

func (ActorAction) TableName() string {
	return "chain.actor_actions"
}
