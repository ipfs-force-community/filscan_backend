package convertor

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
)

type ActorTypeConvertor struct {
}

func (ActorTypeConvertor) ToActorsType(source *actor.ActorsType) (target *po.ActorsType, err error) {
	target = &po.ActorsType{
		ActorID:      source.ActorID,
		ActorAddress: source.ActorAddress,
		ActorType:    source.ActorType,
	}
	return
}
