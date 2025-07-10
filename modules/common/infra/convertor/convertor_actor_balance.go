package convertor

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
)

type ActorBalanceConvertor struct {
}

func (ActorBalanceConvertor) ToActorsBalance(source *actor.RichActor) (target *po.RichActor, err error) {
	target = &po.RichActor{
		Epoch:     source.Epoch,
		ActorID:   source.ActorID,
		ActorType: source.ActorType,
		Balance:   source.Balance,
	}
	return
}
