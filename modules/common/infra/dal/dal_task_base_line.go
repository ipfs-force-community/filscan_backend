package dal

import (
	"context"

	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewBaseLineTaskDal(db *gorm.DB) *BaseLineTaskDal {
	return &BaseLineTaskDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.BaselineTaskRepo = (*BaseLineTaskDal)(nil)

type BaseLineTaskDal struct {
	*_dal.BaseDal
}

func (b BaseLineTaskDal) GetLatestBuiltinActorHeight(ctx context.Context) (epoch chain.Epoch, err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}

	item := new(po.BuiltinActorStatePo)

	err = tx.Order("epoch desc").Limit(1).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}

	epoch = chain.Epoch(item.Epoch)

	return
}

func (b BaseLineTaskDal) SaveBuiltActorStates(ctx context.Context, item ...*po.BuiltinActorStatePo) (err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}
	for _, v := range item {
		err = tx.Create(v).Error
		if err != nil {
			return
		}
	}
	return
}

func (b BaseLineTaskDal) DeleteBuiltActorStates(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.builtin_actor_states where epoch >= ?`, gteEpoch.Int64()).Error
	return
}
