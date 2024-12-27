package dal

import (
	"context"
	"github.com/filecoin-project/go-state-types/builtin"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewStatisticBaseLineBizDal(db *gorm.DB) *StatisticBaseLineBizDal {
	return &StatisticBaseLineBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.StatisticBaseLineBizRepo = (*StatisticBaseLineBizDal)(nil)

type StatisticBaseLineBizDal struct {
	*_dal.BaseDal
}

func (s StatisticBaseLineBizDal) GetBaseLinePowerByPoints(ctx context.Context, points []chain.Epoch) (entities []*bo.BaseLinePower, err error) {
	
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	
	var query []int64
	for _, v := range points {
		query = append(query, v.Int64())
	}
	
	err = tx.Raw(`
		select a.epoch,
		       (a.state ->> 'TotalQualityAdjPower')::decimal   as quality_adj_power,
		       (a.state ->> 'ThisEpochRawBytePower')::decimal  as raw_byte_power,
		       (b.state ->> 'ThisEpochBaselinePower')::decimal as baseline
		from chain.builtin_actor_states a
		         left join chain.builtin_actor_states b on a.epoch = b.epoch and b.actor = ?
		where a.epoch in ?
		  and a.actor = ?
		order by epoch desc
      `, builtin.RewardActorAddr.String(), query, builtin.StoragePowerActorAddr.String()).Find(&entities).Error
	if err != nil {
		return
	}
	
	return
}
