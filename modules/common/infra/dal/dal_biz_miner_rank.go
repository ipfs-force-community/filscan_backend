package dal

import (
	"context"
	"fmt"
	
	"github.com/gozelle/pongo2"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewMinerRankBizDal(db *gorm.DB) *MinerRankBizDal {
	return &MinerRankBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.MinerRankBizRepo = (*MinerRankBizDal)(nil)

type MinerRankBizDal struct {
	*_dal.BaseDal
}

func (b MinerRankBizDal) GetMinerRanks(ctx context.Context, epoch chain.Epoch, query filscan.PagingQuery) (items []*bo.MinerRank, total int64, err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}
	
	field := "quality_adj_power"
	order := "desc"
	if query.Order != nil {
		switch query.Order.Field {
		case "quality_adj_power":
			field = "a.quality_adj_power"
		case "power_increase_24h":
			field = "b.quality_adj_power_change"
		case "block_count":
			field = "b.acc_block_count"
		case "rewards":
			field = "b.acc_reward"
		case "balance":
			field = "a.balance"
		default:
			err = fmt.Errorf("unsupported order field: %s", query.Order.Field)
			return
		}
		switch query.Order.Sort {
		case "desc":
			order = "desc"
		case "asc":
			order = "asc"
		default:
			err = fmt.Errorf("unsupported sort: %s", query.Order.Sort)
			return
		}
	}
	
	tpl, err := pongo2.FromString(`
			SELECT a.epoch,
		       a.miner,
		       a.quality_adj_power,
		       a.quality_adj_power_percent,
		       b.quality_adj_power_change,
		       b.acc_reward,
		       b.acc_reward_percent,
		       b.acc_block_count,
		       b.acc_block_count_percent,
		       a.balance
		FROM "chain".miner_infos a
		         left join chain.miner_stats b on a.epoch = b.epoch and a.miner = b.miner 
		WHERE a.epoch = ? and b.interval = '24h'
		ORDER BY {{ SortField }} {{ SortOrder }}
		offset ? LIMIT ?;
	`)
	if err != nil {
		return
	}
	sql, err := tpl.Execute(map[string]any{
		"SortField": field,
		"SortOrder": order,
	})
	if err != nil {
		return
	}
	
	err = tx.Raw(sql, epoch.Int64(), (query.Index)*query.Limit, query.Limit).Find(&items).Error
	if err != nil {
		return
	}
	
	total, err = b.getEffectiveMiners(ctx, epoch)
	if err != nil {
		return
	}
	
	return
}

func (b MinerRankBizDal) GetMinerPowerRanks(ctx context.Context, epoch, compare chain.Epoch, sectorSize uint64, query filscan.PagingQuery) (items []*bo.MinerPowerRank, total int64, err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}
	
	field := "quality_adj_power_change"
	order := "desc"
	if query.Order != nil {
		switch query.Order.Field {
		case "power_ratio":
			field = "quality_adj_power_change"
		case "quality_power_increase":
			field = "quality_adj_power_change"
		case "quality_adj_power":
			field = "quality_adj_power"
		case "raw_power":
			field = "raw_byte_power"
		default:
			err = fmt.Errorf("unsupported order field: %s", query.Order.Field)
			return
		}
		switch query.Order.Sort {
		case "desc":
			order = "desc"
		case "asc":
			order = "asc"
		default:
			err = fmt.Errorf("unsupported sort: %s", query.Order.Sort)
			return
		}
	}
	
	tpl, err := pongo2.FromString(`with t1 as ( select epoch, miner, quality_adj_power, raw_byte_power, sector_size from chain.miner_infos where epoch = ?)
			select t1.epoch,
			       t2.epoch                                      as prev_epoch,
			       t1.miner,
			       (t1.quality_adj_power - t2.quality_adj_power) as quality_adj_power_change,
			       t1.quality_adj_power,
			       t1.raw_byte_power,
			       t1.sector_size
			from chain.miner_infos t2
			         left join t1 on t1.miner = t2.miner
			where t2.epoch = ?
			  and {{ SectorSize }}
			  and t1.epoch is not null
			order by {{ SortField }} {{ SortOrder }}
	        offset ?
			limit ?;`,
	)
	if err != nil {
		return
	}
	var sql string
	if sectorSize == 0 {
		sql, err = tpl.Execute(map[string]any{
			"SectorSize": "0=?",
			"SortField":  field,
			"SortOrder":  order,
		})
	} else {
		sql, err = tpl.Execute(map[string]any{
			"SectorSize": "t1.sector_size=?",
			"SortField":  field,
			"SortOrder":  order,
		})
	}
	if err != nil {
		return
	}
	
	err = tx.Raw(sql, epoch.Int64(), compare.Int64(), sectorSize, (query.Index)*query.Limit, query.Limit).Find(&items).Error
	if err != nil {
		return
	}
	
	countTpl, err := pongo2.FromString(`
			with t1 as ( select epoch, miner, quality_adj_power, raw_byte_power, sector_size from chain.miner_infos where epoch = ?)
			select count(1)
			from chain.miner_infos t2
			         left join t1 on t1.miner = t2.miner
			where t2.epoch = ?
			  and {{ SectorSize }}
			  and t1.epoch is not null
   `)
	if err != nil {
		return
	}
	var countSql string
	if sectorSize == 0 {
		countSql, err = countTpl.Execute(map[string]any{
			"SectorSize": "0=?",
		})
	} else {
		countSql, err = countTpl.Execute(map[string]any{
			"SectorSize": "t1.sector_size=?",
		})
	}
	if err != nil {
		return
	}
	
	err = tx.Raw(countSql, epoch.Int64(), compare.Int64(), sectorSize).Scan(&total).Error
	if err != nil {
		return
	}
	
	return
}

func (b MinerRankBizDal) GetMinerRewardRanks(ctx context.Context, interval string, epoch chain.Epoch, sectorSize uint64, query filscan.PagingQuery) (items []*bo.MinerRewardRank, total int64, err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}
	field := "acc_reward"
	order := "desc"
	join := "left"
	if query.Order != nil {
		switch query.Order.Field {
		case "rewards":
			field = "a.acc_reward"
		case "block_count":
			field = "a.acc_block_count"
		case "winning_rate":
			field = "a.wining_rate"
		case "quality_adj_power":
			field = "b.quality_adj_power"
			join = "right"
		default:
			err = fmt.Errorf("unsupported order field: %s", query.Order.Field)
			return
		}
		switch query.Order.Sort {
		case "desc":
			order = "desc"
		case "asc":
			order = "asc"
		default:
			err = fmt.Errorf("unsupported sort: %s", query.Order.Sort)
			return
		}
	}
	
	tpl, err := pongo2.FromString(`
		select a.miner, a.acc_reward, a.acc_reward_percent, a.acc_block_count, a.wining_rate, b.quality_adj_power, b.sector_size
		from chain.miner_stats a
				{{ Join }} join chain.miner_infos b on b.epoch = a.epoch and b.miner = a.miner
		where a.epoch = ?
		  and "interval" = ?
		  and {{ SectorSize }}
		order by {{ SortField }} {{ SortOrder }}
		offset ? limit ?
	`)
	if err != nil {
		return
	}
	var sql string
	params := map[string]any{
		"Join":      join,
		"SortField": field,
		"SortOrder": order,
	}
	if sectorSize == 0 {
		params["SectorSize"] = "0=?"
		sql, err = tpl.Execute(params)
	} else {
		params["SectorSize"] = "b.sector_size=?"
		sql, err = tpl.Execute(params)
	}
	if err != nil {
		return
	}
	d := interval
	if d == "1m" {
		d = "30d"
	}
	err = tx.Raw(sql, epoch.Int64(), d, sectorSize, (query.Index)*query.Limit, query.Limit).
		Find(&items).Error
	if err != nil {
		return
	}
	
	table := po.MinerInfo{}
	tx = tx.Table(table.TableName()).Where("epoch = ?", epoch.Int64())
	if sectorSize > 0 {
		tx = tx.Where("sector_size = ?", sectorSize)
	}
	err = tx.Count(&total).Error
	if err != nil {
		return
	}
	
	return
}

func (b MinerRankBizDal) getEffectiveMiners(ctx context.Context, epoch chain.Epoch) (total int64, err error) {
	tx, err := b.DB(ctx)
	if err != nil {
		return
	}
	
	st := po.SyncMinerEpochPo{}
	err = tx.Select("effective_miners").Where("epoch=?", epoch.Int64()).First(&st).Error
	if err != nil {
		return
	}
	total = st.EffectiveMiners
	return
}
