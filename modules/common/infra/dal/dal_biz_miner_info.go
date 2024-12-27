package dal

import (
	"context"

	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewMinerInfoBizDal(db *gorm.DB) *MinerInfoBizDal {
	return &MinerInfoBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type MinerInfoBizDal struct {
	*_dal.BaseDal
}

func (o MinerInfoBizDal) GetMinerInfo(ctx context.Context, ID actor.Id) (item *bo.MinerInfo, err error) {

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

	item = new(bo.MinerInfo)
	err = tx.Raw(`
		SELECT a.*, b.acc_reward, b.acc_block_count, b.acc_win_count
		FROM "chain"."miner_infos" a
		         left join chain.miner_stats b on b.epoch = a.epoch and b.miner = a.miner and b.interval = '30d'
		WHERE a.epoch = ?
		  AND a.miner = ?
		LIMIT 1
	`, epoch.Epoch, ID.Address()).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}

	return
}

func (o MinerInfoBizDal) GetMinerAddressOrNil(ctx context.Context, id chain.SmartAddress) (item *po.MinerLocation, err error) {
	tx, err := o.DB(ctx)
	if err != nil {
		return
	}

	item = new(po.MinerLocation)
	err = tx.Where("miner = ?", id.Address()).First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			item = nil
			err = nil
			return
		}
		return
	}

	return
}

func (o MinerInfoBizDal) GetMinerInfoByEpoch(ctx context.Context, ID actor.Id, epoch chain.Epoch) (item *bo.MinerInfo, err error) {

	tx, err := o.DB(ctx)
	if err != nil {
		return
	}

	item = new(bo.MinerInfo)
	err = tx.Raw(`
		SELECT a.*
		FROM "chain"."miner_infos" a
		WHERE a.epoch >= ?
		  AND a.miner = ?
		LIMIT 1
	`, epoch.Int64(), ID.Address()).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}

	return
}
