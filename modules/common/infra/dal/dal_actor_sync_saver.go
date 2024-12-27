package dal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewActorSyncSaver(db *gorm.DB) *ActorSyncSaver {
	return &ActorSyncSaver{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.ActorSyncSaver = (*ActorSyncSaver)(nil)

type ActorSyncSaver struct {
	*_dal.BaseDal
}

func (a ActorSyncSaver) UpdateActorCreateTime(ctx context.Context, item *po.ActorPo) (err error) {
	
	db, err := a.DB(ctx)
	if err != nil {
		return
	}
	
	err = db.Table(item.TableName()).Where("id=?", item.Id).Update("created_time", item.CreatedTime).Error
	if err != nil {
		return
	}
	
	return
}

func (a ActorSyncSaver) GetNoneCreatedTimeActors(ctx context.Context) (items []*po.ActorPo, err error) {
	db, err := a.DB(ctx)
	if err != nil {
		return
	}
	
	err = db.Where("created_time is null").Find(&items).Error
	if err != nil {
		return
	}
	
	return
}
