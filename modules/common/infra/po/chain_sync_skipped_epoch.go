package po

import "time"

// SyncSkippedEpoch 数据级错误跳过台账：登记因「数据不可解析」等不可恢复错误被跳过的高度。
//
// 语义：该高度已被同步器主动跳过（不执行任务与计算器），后续高度继续同步。
// 与 chain.sync_syncer_epochs 的关系：跳过时仍会写入一条该高度的 tipset 身份行
// （keys/parent_keys），否则链条会出现空洞，一致性检查会把空洞误判为分叉并触发回滚。
//
// DDL 见 migration/33.sync_skipped_epochs.sql。
type SyncSkippedEpoch struct {
	ID            int64     `gorm:"primaryKey"` // 自增主键
	Syncer        string    // 同步器名称
	Epoch         int64     // 被跳过的高度
	ErrorMessage  string    // 触发跳过的错误摘要（已截断）
	Failures      int64     // 触发跳过时的连续失败次数
	FirstFailedAt time.Time // 首次失败时间
	LastFailedAt  time.Time // 末次失败时间
	CreatedAt     time.Time // 首次登记时间（gorm 自动写入）
	UpdatedAt     time.Time // 最近写入时间（gorm 自动维护）
}

func (SyncSkippedEpoch) TableName() string {
	return "chain.sync_skipped_epochs"
}
