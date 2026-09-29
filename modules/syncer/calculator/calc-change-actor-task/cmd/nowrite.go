package actoractionscmd

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// NoWriteChangeActorRepo 是 repository.ChangeActorTask 的「只统计不落库」包装：
// 写方法（chain.actors / chain.actor_actions / chain.actor_balances 的增删）一律**不调用**底层仓储，
// 只往计数器记账；读方法（GetActorBalances / GetActorsByIds / ...）由嵌入接口原样透传。
//
// 安全性质：本类型的写方法体内不存在对底层仓储写方法的调用，因此即使计算器逻辑变化，
// 也不可能经由本类型写入 chain.actors / chain.actor_actions / chain.actor_balances。
type NoWriteChangeActorRepo struct {
	// 嵌入底层仓储：读方法原样透传，只有写方法被本类型覆盖拦下
	repository.ChangeActorTask

	actors   *offlinereplay.Counters // chain.actors
	actions  *offlinereplay.Counters // chain.actor_actions
	balances *offlinereplay.Counters // chain.actor_balances
}

var _ repository.ChangeActorTask = (*NoWriteChangeActorRepo)(nil)

// 派生表名（与 po 的 TableName 一致）
const (
	TableActors        = "chain.actors"
	TableActorActions  = "chain.actor_actions"
	TableActorBalances = "chain.actor_balances"
)

// NewNoWriteChangeActorRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）。
func NewNoWriteChangeActorRepo(inner repository.ChangeActorTask) *NoWriteChangeActorRepo {
	if inner == nil {
		panic("actoractionscmd: NewNoWriteChangeActorRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteChangeActorRepo{
		ChangeActorTask: inner,
		actors:          offlinereplay.NewCounters(TableActors),
		actions:         offlinereplay.NewCounters(TableActorActions),
		balances:        offlinereplay.NewCounters(TableActorBalances),
	}
}

// AddActors 拦下 chain.actors 写入，只计数（行数 = 本次本会写入的行数）
func (r *NoWriteChangeActorRepo) AddActors(_ context.Context, actors []*po.ActorPo) error {
	r.actors.CountWrite(len(actors))
	return nil
}

// AddActorActions 拦下 chain.actor_actions 写入，只计数
func (r *NoWriteChangeActorRepo) AddActorActions(_ context.Context, actions []*po.ActorAction) error {
	r.actions.CountWrite(len(actions))
	return nil
}

// AddActorBalances 拦下 chain.actor_balances 写入，只计数（本命令不跑 change-actor-task，正常不会走到）
func (r *NoWriteChangeActorRepo) AddActorBalances(_ context.Context, balances []*po.ActorBalance) error {
	r.balances.CountWrite(len(balances))
	return nil
}

// DeleteActorsByIds 拦下 chain.actors 按 id 删除（计算器 save() 的「先删后插」路径），只计数
func (r *NoWriteChangeActorRepo) DeleteActorsByIds(_ context.Context, _ []string) error {
	r.actors.CountDelete()
	return nil
}

// DeleteActorBalances 拦下 chain.actor_balances 删除（回滚路径），只计数
func (r *NoWriteChangeActorRepo) DeleteActorBalances(_ context.Context, _ chain.Epoch) error {
	r.balances.CountDelete()
	return nil
}

// DeleteActorActions 拦下 chain.actor_actions 删除（RollBack 路径），只计数
func (r *NoWriteChangeActorRepo) DeleteActorActions(_ context.Context, _ chain.Epoch) error {
	r.actions.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteChangeActorRepo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{r.actions.Snapshot(), r.actors.Snapshot(), r.balances.Snapshot()}
}
