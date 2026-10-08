package dal

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 本文件：chain.reward_stream_recipient_period（受益方按周期归集，支撑「累计已收」）的仓储实现。
// 表结构与口径见 migration/38.reward_stream_recipient_period.sql。

func NewRewardStreamRecipientPeriodDal(db *gorm.DB) *RewardStreamRecipientPeriodDal {
	return &RewardStreamRecipientPeriodDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.RewardStreamRecipientPeriodTask = (*RewardStreamRecipientPeriodDal)(nil)
var _ repository.RewardStreamRecipientPeriodReader = (*RewardStreamRecipientPeriodDal)(nil)

type RewardStreamRecipientPeriodDal struct {
	*_dal.BaseDal
}

// UpsertRewardStreamRecipientPeriods 批量合并写：冲突键 (address, period_start_epoch) 命中时不覆盖，
// 而是合并（幂等，重跑 / 回放不会把大值降回小值）：
//
//	claimed_in_period = GREATEST(已存, excluded)   —— 取值最大的观测（周期内已提单调不减）
//	first_epoch       = LEAST(已存, excluded)
//	last_epoch        = GREATEST(已存, excluded)
//	last_share        = 取「观测高度更大」那一行的值：
//	                    CASE WHEN excluded.last_epoch >= 已存.last_epoch THEN excluded.last_share
//	                         ELSE 已存.last_share END
//	                    （同高度重跑 ⇒ 取新值，同日同值；新观测更旧 ⇒ 保留已存，绝不用陈旧份额盖掉更新的）
//
// 注意：本表**非分区**（见 migration 头注释），所以这里可以安全使用 ON CONFLICT；
// 若日后把它改成按 epoch 分区的 chain 表（带 INSERT 规则），ON CONFLICT 会被 PG 拒绝。
func (s RewardStreamRecipientPeriodDal) UpsertRewardStreamRecipientPeriods(ctx context.Context, items []*po.RewardStreamRecipientPeriod) (err error) {
	if len(items) == 0 {
		return nil
	}
	err = s.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "address"}, {Name: "period_start_epoch"}},
			DoUpdates: clause.Set([]clause.Assignment{
				{
					Column: clause.Column{Name: "claimed_in_period"},
					Value:  gorm.Expr("GREATEST(chain.reward_stream_recipient_period.claimed_in_period, excluded.claimed_in_period)"),
				},
				{
					Column: clause.Column{Name: "first_epoch"},
					Value:  gorm.Expr("LEAST(chain.reward_stream_recipient_period.first_epoch, excluded.first_epoch)"),
				},
				{
					Column: clause.Column{Name: "last_epoch"},
					Value:  gorm.Expr("GREATEST(chain.reward_stream_recipient_period.last_epoch, excluded.last_epoch)"),
				},
				{
					// last_share 只在「新观测不比已存更旧」时替换：确保留存的是最近一次观测到的份额。
					Column: clause.Column{Name: "last_share"},
					Value: gorm.Expr("CASE WHEN excluded.last_epoch >= chain.reward_stream_recipient_period.last_epoch " +
						"THEN excluded.last_share ELSE chain.reward_stream_recipient_period.last_share END"),
				},
			}),
		}).CreateInBatches(items, 100).Error
	})
	return
}

// ListRewardStreamRecipientPeriodsByAddresses 按地址批量取该地址的全部周期行（供上层 SUM 出累计已收）。
// 空地址切片不发 SQL、直接返回 nil。
func (s RewardStreamRecipientPeriodDal) ListRewardStreamRecipientPeriodsByAddresses(ctx context.Context, addresses []string) (items []*po.RewardStreamRecipientPeriod, err error) {
	if len(addresses) == 0 {
		return nil, nil
	}
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("address in ?", addresses).
		Order("address asc, period_start_epoch asc").
		Find(&items).Error
	return
}

// ListRewardStreamRecipientPeriodsByPeriodStart 取某周期起点的全部受益方行（按 address 升序）。
// 供展示层「本周期离场」判定（与快照表并集去重），也用作「本周期是否已开始归集」的探针。
func (s RewardStreamRecipientPeriodDal) ListRewardStreamRecipientPeriodsByPeriodStart(ctx context.Context, periodStart chain.Epoch) (items []*po.RewardStreamRecipientPeriod, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("period_start_epoch = ?", periodStart.Int64()).
		Order("address asc").
		Find(&items).Error
	return
}

// EarliestRewardStreamRecipientPeriodEpoch 全表 MIN(first_epoch)：累计已收「自何高度起有效」。
// 表为空（无任何归集行）⇒ found=false、epoch=0。用 *int64 承接 NULL，避免把「无数据」误当 0。
func (s RewardStreamRecipientPeriodDal) EarliestRewardStreamRecipientPeriodEpoch(ctx context.Context) (epoch chain.Epoch, found bool, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	var agg struct {
		MinFirstEpoch *int64
	}
	err = tx.Model(&po.RewardStreamRecipientPeriod{}).
		Select("min(first_epoch) as min_first_epoch").
		Scan(&agg).Error
	if err != nil {
		return
	}
	if agg.MinFirstEpoch == nil {
		return 0, false, nil
	}
	return chain.Epoch(*agg.MinFirstEpoch), true, nil
}

// DeleteRewardStreamRecipientPeriodsGteEpoch 删除 last_epoch >= gteEpoch 的行（链回滚）。
//
// 边界列是 last_epoch（最近一次观测到该受益方的高度）：回滚到 gteEpoch 之前时，
// 凡「在 gteEpoch 或之后还被观测到」的归集行都不可信，必须删掉靠回放重建。
func (s RewardStreamRecipientPeriodDal) DeleteRewardStreamRecipientPeriodsGteEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.reward_stream_recipient_period where last_epoch >= ?`, gteEpoch.Int64()).Error
	return
}
