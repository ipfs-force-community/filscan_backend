package dal

import (
	"context"

	"github.com/gozelle/pongo2"
	"github.com/pkg/errors"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/debuglog"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewMinerIndicatorDal(db *gorm.DB) *MinerIndicatorBizDal {
	return &MinerIndicatorBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type MinerIndicatorBizDal struct {
	*_dal.BaseDal
}

func (o MinerIndicatorBizDal) GetMinerLucky(ctx context.Context, ID actor.Id, interval *types.IntervalType) (item decimal.Decimal, err error) {
	tx, err := o.DB(ctx)
	if err != nil {
		return
	}
	var epoch po.SyncMinerEpochPo
	err = tx.Select("epoch").Order("epoch desc").First(&epoch).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, nil
		}
		return
	}

	var it string
	switch interval.Value() {
	case types.DAY:
		it = "24h"
	case types.WEEK:
		it = "7d"
	case types.MONTH:
		it = "30d"
	case types.YEAR:
		it = "1y"
	}

	tpl, err := pongo2.FromString(`select b.luck_rate
		from chain.miner_stats b
		where b.miner = ?
		and b.interval = ? {% if it != '1y'%} and b.epoch = ? {% endif %}`)

	ponCtx := pongo2.Context{"it": it}
	res, err := tpl.Execute(ponCtx)
	if err != nil {
		return
	}

	var params []interface{}
	params = append(params, ID.Address(), it)
	if it != "1y" {
		params = append(params, epoch.Epoch)
	}

	err = tx.Raw(res, params...).Find(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return item, nil
		}
	}
	return
}

func (o MinerIndicatorBizDal) GetMinerIndicator(ctx context.Context, ID actor.Id, interval *types.IntervalType) (item *bo.ActorIndicator, err error) {
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
		return
	}

	var it string
	switch interval.Value() {
	case types.DAY:
		it = "24h"
	case types.WEEK:
		it = "7d"
	case types.MONTH:
		it = "30d"
	case types.YEAR:
		it = "1y"
	}

	err = tx.Raw(`
		select a.epoch,
	       a.miner,
	       a.sector_size * b.sector_count_change as seal_power_change,
	       a.quality_adj_power,
	       a.quality_adj_power_rank,
	       b.quality_adj_power_change,
	       b.acc_seal_gas,
	       b.acc_wd_post_gas,
	       b.acc_win_count,
	       b.acc_reward,
	       b.acc_block_count,
	       b.sector_count_change,
	       b.initial_pledge_change,
	       b.luck_rate
	from chain.miner_infos a
	         left join chain.miner_stats b on a.epoch = b.epoch and a.miner = b.miner and b.interval = ?
	where a.epoch = ?
	  and a.miner = ?;
	`, it, epoch.Epoch, ID.Address()).
		Find(&item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	return
}

func (o MinerIndicatorBizDal) GetMinerAccIndicators(ctx context.Context, ID actor.Id, interval *types.IntervalType) (accIndicators *bo.AccIndicators, err error) {
	defer func() {
		debuglog.Logger.Info("accIndicators", accIndicators, err, ID)
	}()

	tx, err := o.DB(ctx)
	if err != nil {
		return
	}

	item := new(po.SyncSyncer)
	err = tx.Where("name = 'chain'").First(item).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return
	}
	endEpoch := chain.Epoch(item.Epoch)
	var startEpoch chain.Epoch
	switch interval.Value() {
	case types.DAY:
		startEpoch = endEpoch - 2880
	case types.WEEK:
		startEpoch = endEpoch - (2880 * 7)
	case types.MONTH:
		startEpoch = endEpoch - (2880 * 30)
	case types.YEAR:
		startEpoch = endEpoch - (2880 * 365)
	}

	var newAccIndicators bo.AccIndicators
	//err = tx.Debug().Select("sum(r.block_count) AS acc_block_count, "+
	//	"sum(r.reward) AS acc_reward, "+
	//	"sum(wc.win_count) AS acc_win_count, "+
	//	"sum(gf.seal_gas) AS acc_seal_gas,"+
	//	"sum(gf.wd_post_gas) AS acc_wd_post_gas").
	//	Table("chain.miner_rewards r").
	//	Joins("LEFT JOIN chain.miner_win_Counts wc ON r.epoch = wc.epoch AND r.miner = wc.miner").
	//	Joins("LEFT JOIN chain.miner_gas_fees gf ON r.epoch = gf.epoch AND r.miner = gf.miner").
	//	Where("r.miner = ? AND r.epoch >= ?", ID, startEpoch).Find(&newAccIndicators).Error
	err = tx.Raw(`
select aa.acc_block_count,
       aa.acc_reward,
       b.acc_win_count,
       c.acc_seal_gas,
       c.acc_wd_post_gas
from (select sum(r.block_count) AS acc_block_count,
             sum(r.reward)      AS acc_reward
      from chain.miner_rewards r
      where epoch >= ?
        and epoch < ?
        and miner = ?) aa,
     (select sum(wc.win_count) AS acc_win_count
      from chain.miner_win_counts wc
     where epoch >= ?
        and epoch < ?
        and miner = ?) b,
     (select sum(gf.seal_gas)    AS acc_seal_gas,
             sum(gf.wd_post_gas) AS acc_wd_post_gas
      from chain.miner_gas_fees gf
      where epoch >= ?
        and epoch < ?
        and miner = ?) c
`, startEpoch.Int64(), endEpoch, ID, startEpoch.Int64(), endEpoch, ID, startEpoch.Int64(), endEpoch, ID).
		Find(&newAccIndicators).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Warnf("fail to get acc indicators of miner %s: %w", ID, err)
			return nil, nil
		}
		return
	}
	accIndicators = &newAccIndicators
	return
}
