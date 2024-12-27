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

func NewChangeActorTaskDal(db *gorm.DB) *ChangeActorTaskDal {
	return &ChangeActorTaskDal{
		BaseDal: _dal.NewBaseDal(db),
	}
}

var _ repository.ChangeActorTask = (*ChangeActorTaskDal)(nil)

type ChangeActorTaskDal struct {
	*_dal.BaseDal
}

func (c ChangeActorTaskDal) GetActorActionsAfterEpoch(ctx context.Context, gteEpoch chain.Epoch) (actions []*po.ActorAction, err error) {

	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Where("epoch >= ?", gteEpoch.Int64()).Find(&actions).Error
	if err != nil {
		return
	}

	return
}

func (c ChangeActorTaskDal) GetActorById(ctx context.Context, id string) (item *po.ActorPo, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	item = new(po.ActorPo)
	err = tx.Where("id = ?", id).First(item).Error
	if err != nil {
		return
	}
	return
}

func (c ChangeActorTaskDal) GetActorByRobust(ctx context.Context, robust string) (item *po.ActorPo, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	item = new(po.ActorPo)
	err = tx.Where("robust = ?", robust).First(item).Error
	if err != nil {
		return
	}
	return
}

func (c ChangeActorTaskDal) GetExistsActors(ctx context.Context, ids []string) (items map[string]string, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	var r []*po.ActorPo
	err = tx.Select("id,type").Where("id in ?", ids).Find(&r).Error
	if err != nil {
		return
	}
	items = map[string]string{}
	for _, v := range r {
		items[v.Id] = v.Type
	}
	return
}

func (c ChangeActorTaskDal) AddActors(ctx context.Context, actors []*po.ActorPo) (err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(actors, 100).Error
	return
}

func (c ChangeActorTaskDal) AddActorActions(ctx context.Context, actors []*po.ActorAction) (err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(actors, 100).Error
	return
}

func (c ChangeActorTaskDal) AddActorBalances(ctx context.Context, balances []*po.ActorBalance) (err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(balances, 100).Error
	return
}

func (c ChangeActorTaskDal) GetActorBalances(ctx context.Context, epoch chain.Epoch) (items []*po.ActorBalance, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch = ?", epoch.Int64()).Find(&items).Error
	return
}

func (c ChangeActorTaskDal) GetActorsByIds(ctx context.Context, ids []string) (items []*po.ActorPo, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("id in ?", ids).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (c ChangeActorTaskDal) DeleteActorsByIds(ctx context.Context, ids []string) (err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	for i := 0; i < len(ids); i += 1000 {
		ml := i + 1000
		if ml > len(ids) {
			ml = len(ids)
		}
		err = tx.Exec("delete from chain.actors where id in ?", ids[i:ml]).Error
		if err != nil {
			return err
		}
	}

	return
}

func (c ChangeActorTaskDal) DeleteActorBalances(ctx context.Context, gteEpoch chain.Epoch) (err error) {

	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Exec(`delete from chain.actor_balances where epoch >= ?`, gteEpoch.Int64()).Error

	return
}

func (c ChangeActorTaskDal) DeleteActorActions(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Exec(`delete from chain.actor_actions where epoch >= ?`, gteEpoch.Int64()).Error

	return
}

func (c ChangeActorTaskDal) GetMinerSizeOrZero(ctx context.Context, miner string) (size int64, err error) {

	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	var epoch int64
	err = tx.Raw(`select epoch from chain.sync_miner_epochs order by epoch desc limit 1`).Scan(&epoch).Error
	if err != nil {
		return
	}

	item := new(po.MinerInfo)
	err = tx.Where("epoch = ? and miner = ?", epoch, miner).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			item = nil
		}
		return
	}
	size = item.SectorSize

	return
}
