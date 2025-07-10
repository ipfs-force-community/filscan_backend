package dal

import (
	"context"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewOwnerBalanceTrendBizDal(db *gorm.DB) *OwnerBalanceTrendBizDal {
	return &OwnerBalanceTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type OwnerBalanceTrendBizDal struct {
	*_dal.BaseDal
}

func (m OwnerBalanceTrendBizDal) GetOwnerBalanceTrend(ctx context.Context, points []chain.Epoch, ownerID actor.Id) (ownerBalanceTrend []*bo.ActorBalanceTrend, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	var epochs []int64
	for _, epoch := range points {
		epochs = append(epochs, epoch.Int64())
	}
	var owners []*po.OwnerInfo
	err = tx.Debug().
		Where("epoch in ? AND owner = ?", epochs, ownerID).
		Order("epoch desc").
		Find(&owners).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	
	for _, owner := range owners {
		ownerBalance := &bo.ActorBalanceTrend{
			Epoch:             owner.Epoch,
			AccountID:         owner.Owner,
			Balance:           owner.Balance,
			AvailableBalance:  &owner.AvailableBalance,
			InitialPledge:     &owner.InitialPledge,
			PreCommitDeposits: &owner.PreCommitDeposits,
			LockedBalance:     &owner.VestingFunds,
		}
		ownerBalanceTrend = append(ownerBalanceTrend, ownerBalance)
	}
	return
}
