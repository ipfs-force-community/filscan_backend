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
	return &RewardStreamRecipientDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.RewardStreamRecipientTask = (*RewardStreamRecipientDal)(nil)

// RewardStreamRecipientDal 只管 chain.reward_stream_recipient_epoch（快照表）；
// 归集表 chain.reward_stream_recipient_period 由 RewardStreamRecipientPeriodDal 单独提供，两者不互相转发。
type RewardStreamRecipientDal struct {
	*_dal.BaseDal
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
