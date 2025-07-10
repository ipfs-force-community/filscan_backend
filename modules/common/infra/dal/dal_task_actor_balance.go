package dal

import (
	"context"
	"github.com/pkg/errors"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewActorBalanceTaskDal(db *gorm.DB) *ActorBalanceTaskDal {
	return &ActorBalanceTaskDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.ActorBalanceTaskRepo = (*ActorBalanceTaskDal)(nil)

type ActorBalanceTaskDal struct {
	*_dal.BaseDal
}

func (a ActorBalanceTaskDal) SaveActorsBalance(ctx context.Context, actorsBalance []*actor.RichActor) (err error) {
	err = a.Exec(ctx, func(tx *gorm.DB) error {
		var items []*po.RichActor
		var conv convertor.ActorBalanceConvertor
		for _, balance := range actorsBalance {
			var item *po.RichActor
			item, err = conv.ToActorsBalance(balance)
			if err != nil {
				return err
			}
			items = append(items, item)
		}
		return tx.CreateInBatches(items, 1000).Error
	})
	return
}

func (a ActorBalanceTaskDal) GetRichAccountRank(ctx context.Context, query filscan.PagingQuery) (result *bo.RichAccountRankList, err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}

	var actorsBalance po.RichActor
	err = tx.Distinct().
		Select("epoch").
		Where("epoch = (select max(epoch) from chain.rich_actors)").
		First(&actorsBalance).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	epoch := chain.Epoch(actorsBalance.Epoch)

	var totalCount int64
	var actorsBalances []*po.RichActor
	if query.Order.Field == "" {
		tx = tx.Model(&actorsBalances).
			Where("epoch = ?", epoch.Int64()).
			Count(&totalCount)
	} else {
		tx = tx.Model(&actorsBalances).
			Where("epoch = ? AND actor_type = ?", epoch.Int64(), query.Order.Field).
			Count(&totalCount)
	}
	tx.Order("balance desc").
		Offset((query.Index) * query.Limit).
		Limit(query.Limit)
	err = tx.Find(&actorsBalances).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}

	var items []*bo.RichAccountRank
	for _, balance := range actorsBalances {
		items = append(items, &bo.RichAccountRank{
			Epoch:   balance.Epoch,
			Actor:   balance.ActorID,
			Balance: balance.Balance,
			Type:    balance.ActorType,
		})
	}
	result = &bo.RichAccountRankList{
		RichAccountRankList: items,
		TotalCount:          totalCount,
	}
	return
}

func (a ActorBalanceTaskDal) DeleteActorsBalance(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.rich_actors where epoch >= ?`, gteEpoch.Int64()).Error
	return
}
