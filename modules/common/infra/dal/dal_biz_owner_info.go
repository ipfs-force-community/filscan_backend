package dal

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shopspring/decimal"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewOwnerInfoBizDal(db *gorm.DB) *OwnerInfoBizDal {
	return &OwnerInfoBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type OwnerInfoBizDal struct {
	*_dal.BaseDal
}

func (o OwnerInfoBizDal) GetOwnerInfo(ctx context.Context, ID actor.Id) (item *bo.OwnerInfo, err error) {
	tx, err := o.DB(ctx)
	if err != nil {
		return
	}
	var epoch po.SyncMinerEpochPo
	err = tx.Select("epoch").Order("epoch desc").First(&epoch).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}

	var (
		stats           []po.OwnerStat
		totalBlockCount int64
		totalReward     decimal.Decimal
		totalWinCount   int64
	)

	err = tx.Raw(`
			SELECT *
			FROM chain.owner_stats
			WHERE epoch <= ? AND owner = ? AND interval = '2880'
			`, epoch.Epoch, ID.Address()).Scan(&stats).Error
	if err != nil {
		return nil, err
	}

	for _, stat := range stats {
		totalBlockCount += stat.AccBlockCount
		totalReward = totalReward.Add(stat.AccReward)
		totalWinCount += stat.AccWinCount
	}

	item = new(bo.OwnerInfo)

	err = tx.Raw(`
		SELECT *
		FROM "chain"."owner_infos"
		WHERE epoch = ? AND owner = ?
		LIMIT 1
	`, epoch.Epoch, ID.Address()).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	item.AccBlockCount = totalBlockCount
	item.AccReward = totalReward
	item.AccWinCount = totalWinCount

	return
}
