package dal

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gorm.io/gorm"
)

func NewActorAggDal(db *gorm.DB) *ActorAggDal {
	return &ActorAggDal{
		ActorGetter: NewActorGetter(db),
	}
}

var _ repository.ActorAggRepo = (*ActorAggDal)(nil)

type ActorAggDal struct {
	*ActorGetter
}
