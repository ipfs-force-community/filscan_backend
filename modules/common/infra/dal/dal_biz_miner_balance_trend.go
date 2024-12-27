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

func NewMinerBalanceTrendBizDal(db *gorm.DB) *MinerBalanceTrendBizDal {
	return &MinerBalanceTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type MinerBalanceTrendBizDal struct {
	*_dal.BaseDal
}

func (m MinerBalanceTrendBizDal) GetMinerBalanceTrend(ctx context.Context, points []chain.Epoch, minerID actor.Id) (minerBalanceTrend []*bo.ActorBalanceTrend, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	
	var epochs []int64
	for _, epoch := range points {
		epochs = append(epochs, epoch.Int64())
	}
	var miners []*po.MinerInfo
	err = tx.Where("epoch in ? AND miner = ?", epochs, minerID).
		Order("epoch desc").
		Find(&miners).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	
	for _, miner := range miners {
		minerBalance := &bo.ActorBalanceTrend{
			Epoch:             miner.Epoch,
			AccountID:         miner.Miner,
			Balance:           miner.Balance,
			AvailableBalance:  &miner.AvailableBalance,
			InitialPledge:     &miner.InitialPledge,
			PreCommitDeposits: &miner.PreCommitDeposits,
			LockedBalance:     &miner.VestingFunds,
		}
		minerBalanceTrend = append(minerBalanceTrend, minerBalance)
	}
	return
}
