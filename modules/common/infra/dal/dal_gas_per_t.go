package dal

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

var _ repository.GasPerTRepo = (*GasPerTDal)(nil)

type GasPerTDal struct {
	*_dal.BaseDal
}

func NewGasPerTDal(db *gorm.DB) *GasPerTDal {
	return &GasPerTDal{BaseDal: _dal.NewBaseDal(db)}
}

func (g GasPerTDal) GetGasPerT(ctx context.Context) (result *bo.GasPerT, err error) {

	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	result = new(bo.GasPerT)
	table := po.SyncMinerEpochPo{}
	err = tx.Table(table.TableName()).Select("epoch").Order("epoch desc").Limit(1).Scan(&result.Epoch).Error
	if err != nil {
		return
	}

	err = tx.Raw(`
		select COALESCE(case when sector_count_change = 0 then 0 else acc_seal_gas / (sector_count_change / 1024 ^ 4) end , 0) as gas_per_t
		from (
		         select sum(sector_count_change) * 34359738368 as sector_count_change,
		                sum(acc_seal_gas)                      as acc_seal_gas
		         from chain.miner_stats
		         where epoch = ?
		           and miner in (select miner from chain.miner_infos where epoch = ? and sector_size = 34359738368)
		           and "interval" = '24h'
		           and sector_count_change > 0
		     ) c;
	`, result.Epoch, result.Epoch).Scan(&result.Gas32G).Error
	if err != nil {
		return
	}

	err = tx.Raw(`
		select COALESCE(case when sector_count_change = 0 then 0 else acc_seal_gas / (sector_count_change / 1024 ^ 4) end , 0) as gas_per_t
		from (
		         select sum(sector_count_change) * 68719476736 as sector_count_change,
		                sum(acc_seal_gas)                      as acc_seal_gas
		         from chain.miner_stats
		         where epoch = ?
		           and miner in (select miner from chain.miner_infos where epoch = ? and sector_size = 68719476736)
		           and "interval" = '24h'
		           and sector_count_change > 0
		     ) c;
	`, result.Epoch, result.Epoch).Scan(&result.Gas64G).Error
	if err != nil {
		return
	}

	return
}
