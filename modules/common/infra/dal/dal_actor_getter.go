package dal

import (
	"context"
	"errors"
	"github.com/filecoin-project/go-state-types/network"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewActorGetter(db *gorm.DB) *ActorGetter {
	return &ActorGetter{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.ActorGetter = (*ActorGetter)(nil)

type ActorGetter struct {
	*_dal.BaseDal
}

func (a ActorGetter) GetActorByIdOrNil(ctx context.Context, nv network.Version, id actor.Id) (item *actor.Actor, err error) {
	db, err := a.DB(ctx)
	if err != nil {
		return
	}
	p := new(po.ActorPo)
	err = db.Where("network = ? and id = ?", nv, id.Address()).First(p).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			p = nil
		}
		return
	}
	return
}

func (a ActorGetter) GetActorInfoByID(ctx context.Context, id actor.Id) (actor *bo.ActorInfo, err error) {
	db, err := a.DB(ctx)
	if err != nil {
		return
	}
	var actorInfo *po.ActorPo
	err = db.Where("id = ?", id.Address()).First(&actorInfo).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			actorInfo = nil
		}
		return
	}
	actor = &bo.ActorInfo{
		Id:          actorInfo.Id,
		Robust:      actorInfo.Robust,
		Type:        actorInfo.Type,
		Code:        actorInfo.Code,
		CreatedTime: actorInfo.CreatedTime,
		LastTxTime:  actorInfo.LastTxTime,
		Balance:     actorInfo.Balance,
	}
	return
}
