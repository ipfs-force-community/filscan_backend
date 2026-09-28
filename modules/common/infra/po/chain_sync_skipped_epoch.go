package po

import "time"

// SyncSkippedEpoch 跳过台账：登记因「不可恢复错误」被同步器主动跳过的高度（或整个区间）。
//
// 两类记录：
//   - 单高度（原有语义）：Epoch = 被跳过的高度，SkippedFrom / SkippedTo 为 NULL；
//     error_class = "data"（数据级错误，例如聚合器 code:1 / 响应体无法解码）。
//   - 区间（节点侧历史状态不可用，一次跳过几万个连续高度）：Epoch = *SkippedFrom = 区间起点，
//     SkippedTo = 区间终点（含两端），error_class = "unrecoverable-state"。
//
// 语义：这些高度已被同步器主动跳过（不执行任务与计算器），后续高度继续同步。
// 与 chain.sync_syncer_epochs 的关系：单高度跳过时仍会写入一条该高度的 tipset 身份行
// （keys/parent_keys），否则链条会出现空洞，一致性检查会把空洞误判为分叉并触发回滚；
// 区间跳是**有意**留下整段空洞（这正是它要解决的问题：那一段高度在节点侧取不到状态），
// 由本表的区间行留痕，运维可据此把 chain.sync_syncers.epoch 改回区间起点重跑（可逆）。
//
// DDL 见 migration/33.sync_skipped_epochs.sql（建表）与
// migration/34.sync_skipped_epochs_ranges.sql（新增 error_class / skipped_from / skipped_to）。
type SyncSkippedEpoch struct {
	ID            int64     `gorm:"primaryKey"` // 自增主键
	Syncer        string    // 同步器名称
	Epoch         int64     // 被跳过的高度；区间记录时为区间起点（= *SkippedFrom）
	ErrorMessage  string    // 触发跳过的错误摘要（已截断）
	Failures      int64     // 触发跳过时的连续失败次数
	FirstFailedAt time.Time // 首次失败时间
	LastFailedAt  time.Time // 末次失败时间
	// ErrorClass 触发跳过的错误类别：data / unrecoverable-state；老记录为 NULL，语义等同 data。
	ErrorClass *string
	// SkippedFrom / SkippedTo 仅「区间跳」写入：被跳过的高度区间 [SkippedFrom, SkippedTo]（含两端）。
	// 老记录与单高度跳过的记录为 NULL（语义等同 SkippedFrom = SkippedTo = Epoch）。
	SkippedFrom *int64
	SkippedTo   *int64
	CreatedAt   time.Time // 首次登记时间（gorm 自动写入）
	UpdatedAt   time.Time // 最近写入时间（gorm 自动维护）
}

func (SyncSkippedEpoch) TableName() string {
	return "chain.sync_skipped_epochs"
}

// IsRange 是否为「区间跳」记录（覆盖一段连续高度，而非单个高度）
func (e *SyncSkippedEpoch) IsRange() bool {
	return e != nil && e.SkippedFrom != nil && e.SkippedTo != nil && *e.SkippedTo > *e.SkippedFrom
}
