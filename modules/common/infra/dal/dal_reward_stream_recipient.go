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

// 本文件：chain.reward_stream_recipient_epoch（f02 奖励流受益方按高度快照）的仓储实现。
// 表结构与口径见 migration/37.reward_stream_recipient_epoch.sql。

func NewRewardStreamRecipientDal(db *gorm.DB) *RewardStreamRecipientDal {
	return &RewardStreamRecipientDal{
		BaseDal: _dal.NewBaseDal(db),
		// 同一 db 构造归集表 dal：展示层把两张「受益方来源」表（按高度快照 + 按周期归集）
		// 交给同一个注入对象，避免改动 biz_statistic.go 的生产装配（见下方读方法转发）。
		period: NewRewardStreamRecipientPeriodDal(db),
	}
}

var _ repository.RewardStreamRecipientTask = (*RewardStreamRecipientDal)(nil)

type RewardStreamRecipientDal struct {
	*_dal.BaseDal
	// period 归集表（chain.reward_stream_recipient_period）dal：本类型把它的三个只读方法转发出去，
	// 使注入的对象同时满足展示层「快照 + 归集」两路只读来源（展示层窄接口定义在 biz 层）。
	period *RewardStreamRecipientPeriodDal
}

// 以下三个只读方法把归集表读路径转发给 period dal：展示层用同一个注入对象同时取「快照」与「归集」。
// 归集表（只增不减、跨周期保留）是「累计已收」与「本周期离场」的可靠来源；快照表仅作 last_share 补充。

// ListRewardStreamRecipientPeriodsByAddresses 见 RewardStreamRecipientPeriodDal 同名方法。
func (s RewardStreamRecipientDal) ListRewardStreamRecipientPeriodsByAddresses(ctx context.Context, addresses []string) ([]*po.RewardStreamRecipientPeriod, error) {
	return s.period.ListRewardStreamRecipientPeriodsByAddresses(ctx, addresses)
}

// ListRewardStreamRecipientPeriodsByPeriodStart 见 RewardStreamRecipientPeriodDal 同名方法。
func (s RewardStreamRecipientDal) ListRewardStreamRecipientPeriodsByPeriodStart(ctx context.Context, periodStart chain.Epoch) ([]*po.RewardStreamRecipientPeriod, error) {
	return s.period.ListRewardStreamRecipientPeriodsByPeriodStart(ctx, periodStart)
}

// EarliestRewardStreamRecipientPeriodEpoch 见 RewardStreamRecipientPeriodDal 同名方法。
func (s RewardStreamRecipientDal) EarliestRewardStreamRecipientPeriodEpoch(ctx context.Context) (chain.Epoch, bool, error) {
	return s.period.EarliestRewardStreamRecipientPeriodEpoch(ctx)
}

// SaveRewardStreamRecipients 批量 upsert：冲突键 (epoch, address)，命中即覆盖
// share / payable / claimed_period / tombstone —— 同一高度重跑后表内容与只跑一次完全相同。
//
// 注意：本表**非分区**（见 migration 头注释），所以这里可以安全使用 ON CONFLICT；
// 若日后把它改成按 epoch 分区的 chain 表（带 INSERT 规则），ON CONFLICT 会被 PG 拒绝。
func (s RewardStreamRecipientDal) SaveRewardStreamRecipients(ctx context.Context, items []*po.RewardStreamRecipientEpoch) (err error) {
	if len(items) == 0 {
		return nil
	}
	err = s.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "epoch"}, {Name: "address"}},
			DoUpdates: clause.AssignmentColumns([]string{"share", "payable", "claimed_period", "tombstone"}),
		}).CreateInBatches(items, 100).Error
	})
	return
}

// ListRewardStreamRecipientsByEpochRange 按 epoch 区间（左闭右闭）取快照行。
func (s RewardStreamRecipientDal) ListRewardStreamRecipientsByEpochRange(ctx context.Context, epochs chain.LCRCRange) (items []*po.RewardStreamRecipientEpoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch >= ? and epoch <= ?", epochs.GteBegin.Int64(), epochs.LteEnd.Int64()).
		Order("epoch asc, address asc").
		Find(&items).Error
	return
}

// DeleteRewardStreamRecipientsGteEpoch 删除 epoch >= gteEpoch 的行（链回滚）。
func (s RewardStreamRecipientDal) DeleteRewardStreamRecipientsGteEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.reward_stream_recipient_epoch where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

// DeleteRewardStreamRecipientsLteEpoch 删除 epoch <= lteEpoch 的行（历史清理）。
func (s RewardStreamRecipientDal) DeleteRewardStreamRecipientsLteEpoch(ctx context.Context, lteEpoch chain.Epoch) (err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.reward_stream_recipient_epoch where epoch <= ?`, lteEpoch.Int64()).Error
	return
}
