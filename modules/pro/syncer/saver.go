package prosyncer

import (
	"context"

	"github.com/pkg/errors"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

type iSaver interface {
	SaveMinerFunds(ctx context.Context, items []*propo.MinerFund) (err error)
	SaveMinerBalances(ctx context.Context, items []*propo.MinerBalance) (err error)
	SaveMinerInfos(ctx context.Context, items []*propo.MinerInfo) (err error)
	CountMinerDcs(ctx context.Context, epoch int64) (count int64, err error)
	HasMinerDc(ctx context.Context, miner string, epoch chain.Epoch) (ok bool, err error)
	SaveMineDc(ctx context.Context, item *propo.MinerDc) (err error)
	SaveMinerSectors(ctx context.Context, items []*propo.MinerSector) (err error)
	UpdateMinerAggRewards(ctx context.Context, items []*propo.MinerAggReward) (err error)
	DeleteMinerSectorsBeforeEpoch(ctx context.Context, epoch chain.Epoch) (err error)
	RollbackMinerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error)
	RollbackMinerFunds(ctx context.Context, gteEpoch chain.Epoch) (err error)
	RollbackMinerBalances(ctx context.Context, gteEpoch chain.Epoch) (err error)
	RollbackMinerSectors(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

func newSaver(db *gorm.DB) *saver {
	return &saver{BaseDal: _dal.NewBaseDal(db)}
}

var _ iSaver = (*saver)(nil)

type saver struct {
	*_dal.BaseDal
}

func (s saver) HasMinerDc(ctx context.Context, miner string, epoch chain.Epoch) (exist bool, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}

	var item *propo.MinerDc

	err = tx.Where("epoch = ? and miner = ?", epoch.Int64(), miner).First(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}

	exist = true

	return
}

func (s saver) SaveMinerInfos(ctx context.Context, items []*propo.MinerInfo) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(items, 200).Error
	if err != nil {
		return
	}
	return
}

func (s saver) SaveMineDc(ctx context.Context, item *propo.MinerDc) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Create(item).Error
	if err != nil {
		return
	}
	return
}

func (s saver) SaveMinerSectors(ctx context.Context, items []*propo.MinerSector) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(items, 200).Error
	if err != nil {
		return
	}
	return
}

func (s saver) SaveMinerFunds(ctx context.Context, items []*propo.MinerFund) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(items, 200).Error
	if err != nil {
		return
	}
	return
}

func (s saver) SaveMinerBalances(ctx context.Context, items []*propo.MinerBalance) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.CreateInBatches(items, 200).Error
	if err != nil {
		return
	}
	return
}

func (s saver) UpdateMinerAggRewards(ctx context.Context, items []*propo.MinerAggReward) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	for _, v := range items {
		err = tx.Table(v.TableName()).Where("miner = ?").Updates(map[string]any{
			"agg_reward":    v.AggReward,
			"agg_block":     v.AggBlock,
			"agg_win_count": v.AggWinCount,
		}).Error
		if err != nil {
			return
		}
	}
	return
}

func (s saver) RollbackMinerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from pro.miner_infos where epoch >= ? `, gteEpoch.Int64()).Error
	return
}

func (s saver) RollbackMinerFunds(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from pro.miner_funds where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (s saver) RollbackMinerBalances(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from pro.miner_balances where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (s saver) RollbackMinerSectors(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from pro.miner_sectors where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (s saver) DeleteMinerSectorsBeforeEpoch(ctx context.Context, epoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from pro.miner_sectors where epoch < ?`, epoch.Int64()).Error
	return
}

func (s saver) CountMinerDcs(ctx context.Context, epoch int64) (count int64, err error) {

	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`select count(1) from pro.miner_dcs where epoch = ?`, epoch).Scan(&count).Error
	if err != nil {
		return
	}
	return
}
