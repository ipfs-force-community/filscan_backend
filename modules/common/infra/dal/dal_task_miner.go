package dal

import (
	"context"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

func NewMinerTaskDal(db *gorm.DB) *MinerTaskDal {
	return &MinerTaskDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.MinerTask = (*MinerTaskDal)(nil)

type MinerTaskDal struct {
	*_dal.BaseDal
}

func (m MinerTaskDal) SaveAbsPower(ctx context.Context, powerIncrease, powerLoss decimal.Decimal, epoch int64) error {
	tx, err := m.DB(ctx)
	if err != nil {
		return err
	}
	return tx.Save(&po.AbsPowerChange{
		Epoch:         epoch,
		PowerIncrease: powerIncrease,
		PowerLoss:     powerLoss,
	}).Error
}

func (m MinerTaskDal) GetMinersAccWinCount(ctx context.Context, epochs chain.LORCRange) (items []*bo.AccWinCount, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
		select
		       miner,
		       sum(win_count) as win_count		
		from chain.miner_win_counts
		where epoch >= ?
		  and epoch < ?
		group by miner;
	`, epochs.GtBegin.Int64(), epochs.LteEnd.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerTaskDal) GetMinersAccGasFees(ctx context.Context, epochs chain.LORCRange) (fees []*bo.AccGasFee, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
		select
		       miner,
		       sum(pre_agg) as pre_agg,
		       sum(prove_agg) as prove_agg,
		       sum(sector_gas) as sector_gas,
		       sum(wd_post_gas) as wd_post_gas,
		       sum(seal_gas) as seal_gas
		from chain.miner_gas_fees
		where epoch >= ?
		  and epoch < ?
		group by miner;
	`, epochs.GtBegin.Int64(), epochs.LteEnd.Int64()).Find(&fees).Error
	if err != nil {
		return
	}

	return
}

func (m MinerTaskDal) GetMinersAccRewards(ctx context.Context, epochs chain.LORCRange) (rewards []*bo.AccReward, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
		select
		       miner,
		       sum(reward) as reward,
		       sum(block_count) as block_count
		from chain.miner_rewards
		where epoch >= ?
		  and epoch < ?
		group by miner;
	`, epochs.GtBegin.Int64(), epochs.LteEnd.Int64()).Find(&rewards).Error
	if err != nil {
		return
	}

	return
}

func (m MinerTaskDal) GetMinerInfosByEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.MinerInfo, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch = ?", epoch.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerTaskDal) GetOwnerInfosByEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.OwnerInfo, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch = ?", epoch.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerTaskDal) SaveSyncMinerEpochPo(ctx context.Context, item *po.SyncMinerEpochPo) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Create(item).Error
	})
	return
}

func (m MinerTaskDal) SaveOwnerInfos(ctx context.Context, infos []*po.OwnerInfo) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

func (m MinerTaskDal) SaveMinerInfos(ctx context.Context, infos []*po.MinerInfo) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

func (m MinerTaskDal) SaveOwnerStats(ctx context.Context, stats []*po.OwnerStat) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(stats, 100).Error
	})
	return
}

func (m MinerTaskDal) SaveMinerStats(ctx context.Context, stats []*po.MinerStat) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(stats, 100).Error
	})
	return
}

func (m MinerTaskDal) DeleteMinerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_infos where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteOwnerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.owner_infos where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteSyncMinerEpochs(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.sync_miner_epochs where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteMinerStats(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_stats where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteOwnerStats(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.owner_stats where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteMinerStatsBeforeEpoch(ctx context.Context, ltEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_stats where epoch < ? and interval != '2880'`, ltEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteOwnerStatsBeforeEpoch(ctx context.Context, ltEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.owner_stats where epoch < ? and interval != '2880'`, ltEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteAbsPower(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.abs_power_change where epoch >= ?`, gteEpoch.Int64()).Error
	return
}
