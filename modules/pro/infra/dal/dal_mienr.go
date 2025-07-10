package prodal

import (
	"context"

	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewMinerInfoDal(db *gorm.DB) *MinerInfoDal {
	return &MinerInfoDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.MinerRepo = (*MinerInfoDal)(nil)

type MinerInfoDal struct {
	*_dal.BaseDal
}

func (m MinerInfoDal) GetProInfoEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`select max(epoch) from pro.miner_infos`).Scan(&epoch).Error
	if err != nil {
		return
	}
	return
}

func (m MinerInfoDal) GetMinerInfos(ctx context.Context, epoch int64, miners []string) (infos map[string]*propo.MinerInfo, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	var items []*propo.MinerInfo
	err = tx.Where("epoch = ? and miner in ?", epoch, miners).Find(&items).Error
	if err != nil {
		return
	}
	infos = map[string]*propo.MinerInfo{}
	for _, v := range items {
		infos[v.Miner] = v
	}
	return
}

func (m MinerInfoDal) GetMinerBalances(ctx context.Context, epoch int64, miners []string) (balances map[string]*propo.MinerBalance, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	var items []*propo.MinerBalance
	err = tx.Debug().Where("epoch = ? and miner in ? and type='miner'", epoch, miners).Find(&items).Error
	if err != nil {
		return
	}
	balances = map[string]*propo.MinerBalance{}
	for _, v := range items {
		balances[v.Miner] = v
	}
	return
}

func (m MinerInfoDal) GetMinerFunds(ctx context.Context, epochs chain.LORCRange, miners []string) (fees map[string]*propo.MinerFund, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	var items []*propo.MinerFund
	err = tx.Raw(`
	SELECT miner, 
	   sum(income)      as income,
       sum(outlay)     as outlay,
       sum(seal_gas)    as seal_gas,
       sum(wd_post_gas) as wd_post_gas,
       sum(wd_post_gas) as wd_post_gas,
       sum(deal_gas)    as deal_gas,
       sum(total_gas)   as total_gas,
       sum(penalty)     as penalty,
       sum(reward)      as reward,
       sum(block_count) as block_count,
       sum(win_count)   as win_count,
       sum(other_gas)   as other_gas,
       sum(pre_agg)     as pre_agg,
       sum(pro_agg)     as pro_agg
FROM "pro"."miner_funds"
WHERE epoch > ?
  and epoch <= ?
  and miner in ?
group by miner;
`, epochs.GtBegin.Int64(), epochs.LteEnd.Int64(), miners).Find(&items).Error
	if err != nil {
		return
	}
	fees = map[string]*propo.MinerFund{}
	for _, v := range items {
		v.SealGas = v.SealGas.Add(v.PreAgg).Add(v.ProAgg)
		v.TotalGas = v.TotalGas.Add(v.PreAgg).Add(v.ProAgg)
		fees[v.Miner] = v

	}
	return
}

func (m MinerInfoDal) GetMinerAccReward(ctx context.Context, miners string) (r map[string]decimal.Decimal, err error) {
	return
}

func (m MinerInfoDal) GetMinersSectors(ctx context.Context, epoch int64, miners []string) (sectors []*propo.MinerSector, err error) {

	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Where(`epoch = ? and miner in ?`, epoch, miners).Find(&sectors).Error
	if err != nil {
		return
	}
	return
}

func (m MinerInfoDal) GetSyncEpoch(ctx context.Context) (epoch int64, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	table := propo.MinerFund{}
	err = tx.Table(table.TableName()).Select("epoch").Order("epoch desc").Limit(1).Scan(&epoch).Error
	if err != nil {
		return
	}
	return
}
