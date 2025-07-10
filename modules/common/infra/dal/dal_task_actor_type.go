package dal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewActorTypeTaskDal(db *gorm.DB) *ActorTypeTaskDal {
	return &ActorTypeTaskDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.ActorTypeTaskRepo = (*ActorTypeTaskDal)(nil)

type ActorTypeTaskDal struct {
	*_dal.BaseDal
}

func (a ActorTypeTaskDal) SaveActorsType(ctx context.Context, actorsType []*actor.ActorsType) (err error) {
	err = a.Exec(ctx, func(tx *gorm.DB) error {
		var items []*po.ActorsType
		var conv convertor.ActorTypeConvertor
		for _, actorType := range actorsType {
			var item *po.ActorsType
			item, err = conv.ToActorsType(actorType)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		err = tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "actor_id"},
				{Name: "actor_address"},
				{Name: "actor_type"}},
			DoNothing: true}).
			CreateInBatches(items, 100).Error
		return err
	})
	return
}

func (a ActorTypeTaskDal) GetActorType(ctx context.Context, actorID chain.SmartAddress) (result *bo.ActorType, err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}
	var actorsType *po.ActorsType
	tx.Where("actor_id = ? OR actor_address = ?", actorID.Address(), actorID.Address()).
		First(&actorsType)
	result = &bo.ActorType{
		ActorID:      actorsType.ActorID,
		ActorAddress: actorsType.ActorAddress,
		ActorType:    actorsType.ActorType,
	}
	return
}
