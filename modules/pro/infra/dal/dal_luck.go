package prodal

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewLuckDal(db *gorm.DB) *LuckDal {
	return &LuckDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.LuckRepo = (*LuckDal)(nil)

type LuckDal struct {
	*_dal.BaseDal
}

func (l LuckDal) QueryMinerLucks(ctx context.Context, miners []string, epoch int64) (m map[string]*prorepo.Luck, err error) {
	
	tx, err := l.DB(ctx)
	if err != nil {
		return
	}
	
	var items []po.MinerStat
	
	err = tx.Debug().Select("miner,interval,luck_rate").Where("epoch = ? and miner in ?", epoch, miners).Find(&items).Error
	if err != nil {
		return
	}
	m = map[string]*prorepo.Luck{}
	
	for _, v := range items {
		if _, ok := m[v.Miner]; !ok {
			m[v.Miner] = &prorepo.Luck{
				Miner: v.Miner,
			}
		}
		switch v.Interval {
		case "24h":
			m[v.Miner].Luck24h = v.LuckRate
		case "7d":
			m[v.Miner].Luck7d = v.LuckRate
		case "30d":
			m[v.Miner].Luck30d = v.LuckRate
		}
	}
	
	return
}
