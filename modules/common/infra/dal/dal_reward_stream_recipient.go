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

// latestRewardStreamRecipientEpochPerAddressSQL 构造「每个 address 只取 epoch 最大的那一行」的聚合 SQL。
//
// 为什么是「全局每地址最后一行」而不是「本周期范围」：本表可被框架 HistoryClear 按高度修剪，
// 但 tombstone 遗留欠款收款人（如 2026-10-13 cali 实测的 t0200206）只在快照表里有行、行还都在更早周期，
// 归集表完全无它的行 —— 只取本周期范围会漏掉它，页面会少一行。见 ListLatestRewardStreamRecipientEpochPerAddress。
//
// 表名取自 PO.TableName()（不硬编码，改名后不会指向错表）。单独成函数便于测试对 SQL 结构做断言：
// DISTINCT ON (address) + ORDER BY address asc, epoch desc ⇒ 每地址恰好回 epoch 最大的一行；
// 只有全局上界 epoch <= ?，**没有**任何周期下界（否则就退化成「本周期内最大」，会漏人）。
func latestRewardStreamRecipientEpochPerAddressSQL() string {
	return `select distinct on (address)
		epoch, address, share, payable, claimed_period, tombstone
	from ` + po.RewardStreamRecipientEpoch{}.TableName() + `
	where epoch <= ?
	order by address asc, epoch desc`
}

// ListLatestRewardStreamRecipientEpochPerAddress 对每个受益地址只取「epoch 最大」的那一行快照
// （全局、限于 epoch <= maxEpoch），**每地址最多一行、单次聚合查询**（禁 N+1：不准按地址逐个查行）。
//
// 语义 = 「每个地址在全部历史上最后一行」：改用这条后，只在快照表里、且行都在更早周期的地址
// （归集表无其行）也能被展示层认出并标 departed=true。走 migration/37 的
// (address, epoch) 索引做前缀定序。空结果返回 nil（不是错误）。
func (s RewardStreamRecipientDal) ListLatestRewardStreamRecipientEpochPerAddress(ctx context.Context, maxEpoch chain.Epoch) (items []*po.RewardStreamRecipientEpoch, err error) {
	tx, err := s.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(latestRewardStreamRecipientEpochPerAddressSQL(), maxEpoch.Int64()).Find(&items).Error
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
