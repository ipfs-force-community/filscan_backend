package dal

import (
	"context"
	"fmt"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewMinerPowerTrendBizDal(db *gorm.DB) *MinerPowerTrendBizDal {
	return &MinerPowerTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type MinerPowerTrendBizDal struct {
	*_dal.BaseDal
}

func (m MinerPowerTrendBizDal) GetMinerPowerTrend(ctx context.Context, points []chain.Epoch, minerID actor.Id) (minerPowerTrend []*bo.ActorPowerTrend, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	var epochs []int64
	for _, epoch := range points {
		epochs = append(epochs, epoch.Int64())
	}

	if len(epochs) < 2 {
		err = fmt.Errorf("expect epochs > 2")
		return
	}

	gap := epochs[1] - epochs[0]

	err = tx.Raw(`
			select a.epoch,a.miner as account_id, a.quality_adj_power as power, (a.quality_adj_power - b.quality_adj_power ) as power_change
			from chain.miner_infos a
			         left join chain.miner_infos b on b.epoch = a.epoch - ? and a.miner = b.miner
			where a.epoch in ?
			  and a.miner = ?
			order by a.epoch desc;
		`, gap, epochs, minerID.Address()).Find(&minerPowerTrend).Error
	if err != nil {
		return
	}

	for _, v := range minerPowerTrend {
		v.BlockTime = chain.Epoch(v.Epoch).Unix()
	}

	return
}
