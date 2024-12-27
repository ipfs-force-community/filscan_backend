package prodal

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	probo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewCapitalDal(db *gorm.DB) *CapitalDal {
	return &CapitalDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.CapitalRepo = (*CapitalDal)(nil)

type CapitalDal struct {
	*_dal.BaseDal
}

func (c CapitalDal) GetAddressRank(ctx context.Context) (result *probo.RichAccountRankList, err error) {
	tx, err := c.DB(ctx)
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
	tx = tx.Model(&actorsBalances).
		Where("epoch = ?", epoch.Int64()).
		Count(&totalCount)
	tx.Order("balance desc")
	err = tx.Find(&actorsBalances).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}

	var items []*probo.RichAccountRank
	for _, balance := range actorsBalances {
		items = append(items, &probo.RichAccountRank{
			Actor:   balance.ActorID,
			Balance: balance.Balance,
			Type:    balance.ActorType,
		})
	}
	result = &probo.RichAccountRankList{
		RichAccountRankList: items,
	}
	return
}

func (c CapitalDal) GetLatestBalanceBeforeEpoch(ctx context.Context, address string, epoch *chain.Epoch) (balance decimal.Decimal, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	var miner *po.ActorBalance
	err = tx.Where("actor_id = ? AND epoch > ?", address, epoch.Int64()).
		Order("epoch desc").
		First(&miner).Error
	if err != nil {
		return
	}
	balance = miner.Balance
	return
}

func (c CapitalDal) GetLatestBalanceAfterEpoch(ctx context.Context, address string, epoch *chain.Epoch) (balance decimal.Decimal, err error) {
	tx, err := c.DB(ctx)
	if err != nil {
		return
	}

	var miner *po.ActorBalance
	err = tx.Where("actor_id = ? AND epoch <= ?", address, epoch.Int64()).
		Order("epoch desc").
		First(&miner).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return decimal.Zero, nil
		}
		return
	}
	balance = miner.Balance
	return
}
