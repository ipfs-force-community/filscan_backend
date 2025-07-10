package po

type ActorsType struct {
	ActorID      string
	ActorAddress string
	ActorType    string
}

func (ActorsType) TableName() string {
	return "chain.actors_type"
}
