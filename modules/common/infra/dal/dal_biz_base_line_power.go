package dal

// 注意：本文件 GetBaseLinePowerByPoints 的自连接 SQL 里，内层 b 必须带上常量区间谓词
// `b.epoch between ? and ?`。加它是为了让规划期就能对 chain.builtin_actor_states（300+ 分区）
// 做分区裁剪：否则自连接的规划代价会被估到 ~1.3M（越过 PG 的 jit_optimize_above_cost=500000），
// 触发 JIT 编译上千个函数，主网 30d 档曾因此要 11s（真正执行仅 ~64ms）。

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

	// 从本次查询的高度里现算 min/max，给内层 b 做常量区间谓词（语义等价：b.epoch = a.epoch 且 a.epoch ∈ [min,max]）。
	// 目的是让 PG 在规划期对分区表做裁剪、把代价压到 JIT 阈值以下。
	var minEpoch, maxEpoch int64
	if len(query) > 0 {
		minEpoch, maxEpoch = query[0], query[0]
		for _, v := range query[1:] {
			if v < minEpoch {
				minEpoch = v
			}
			if v > maxEpoch {
				maxEpoch = v
			}
		}
	}

	err = tx.Raw(`
		select a.epoch,
		       (a.state ->> 'TotalQualityAdjPower')::decimal   as quality_adj_power,
		       (a.state ->> 'ThisEpochRawBytePower')::decimal  as raw_byte_power,
		       (b.state ->> 'ThisEpochBaselinePower')::decimal as baseline
		from chain.builtin_actor_states a
		         left join chain.builtin_actor_states b
		                   on a.epoch = b.epoch and b.actor = ?
		                  and b.epoch between ? and ?
		where a.epoch in ?
		  and a.actor = ?
		order by epoch desc
      `, builtin.RewardActorAddr.String(), minEpoch, maxEpoch, query, builtin.StoragePowerActorAddr.String()).Find(&entities).Error
	if err != nil {
		return
	}

	return
}
