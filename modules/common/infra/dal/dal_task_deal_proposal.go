package dal

import (
	"context"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/gorm"
	
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

var _ repository.DealProposalTaskRepo = (*DealProposalTaskDal)(nil)

func (d DealProposalTaskDal) SaveDealProposals(ctx context.Context, items ...*po.DealProposalPo) (err error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return
	}
	return tx.CreateInBatches(items, 100).Error
}

type DealProposalTaskDal struct {
	*_dal.BaseDal
}

func NewDealProposalTaskDal(db *gorm.DB) *DealProposalTaskDal {
	return &DealProposalTaskDal{
		BaseDal: _dal.NewBaseDal(db),
	}
}

func (d DealProposalTaskDal) GetCidByDeal(ctx context.Context, dealID int64) (item *po.DealProposalPo, err error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("deal_id = ?", dealID).
		First(&item).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	return
}

func (d DealProposalTaskDal) DeleteDealProposals(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := d.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.deal_proposals where epoch > ?`, gteEpoch.Int64()).Error
	return
}
