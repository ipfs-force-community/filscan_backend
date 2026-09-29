package baselineactorscmd

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// NoWriteBaselineRepo 是 repository.BaselineTaskRepo 的「只统计不落库」包装：
// 写方法（chain.builtin_actor_states 的增删）一律**不调用**底层仓储，只往计数器记账；
// 读方法（GetLatestBuiltinActorHeight）由嵌入接口原样透传。
//
// 安全性质：本类型的写方法体内不存在对底层仓储写方法的调用，因此即使任务逻辑变化，
// 也不可能经由本类型写入 chain.builtin_actor_states。
type NoWriteBaselineRepo struct {
	// 嵌入底层仓储：读方法原样透传，只有写方法被本类型覆盖拦下
	repository.BaselineTaskRepo

	states *offlinereplay.Counters // chain.builtin_actor_states
}

var _ repository.BaselineTaskRepo = (*NoWriteBaselineRepo)(nil)

// 派生表名（与 po 的 TableName 一致）
const TableBuiltinActorStates = "chain.builtin_actor_states"

// NewNoWriteBaselineRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）
func NewNoWriteBaselineRepo(inner repository.BaselineTaskRepo) *NoWriteBaselineRepo {
	if inner == nil {
		panic("baselineactorscmd: NewNoWriteBaselineRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteBaselineRepo{
		BaselineTaskRepo: inner,
		states:           offlinereplay.NewCounters(TableBuiltinActorStates),
	}
}

// SaveBuiltActorStates 拦下 chain.builtin_actor_states 写入，只计数（行数 = 本次本会写入的行数）
func (r *NoWriteBaselineRepo) SaveBuiltActorStates(_ context.Context, item ...*po.BuiltinActorStatePo) error {
	r.states.CountWrite(len(item))
	return nil
}

// DeleteBuiltActorStates 拦下 chain.builtin_actor_states 删除（RollBack 路径），只计数
func (r *NoWriteBaselineRepo) DeleteBuiltActorStates(_ context.Context, _ chain.Epoch) error {
	r.states.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteBaselineRepo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{r.states.Snapshot()}
}
