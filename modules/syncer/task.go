package syncer

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type Namer interface {
	Name() string
}

var _ Namer = (*EmptyName)(nil)

type EmptyName struct {
}

func (e EmptyName) Name() string {
	return ""
}

type base interface {
	// Name 任务唯一名称，用于数据库任务标识、日志打印等
	Name() string
	// RollBack 执行回滚操作
	// gteEpoch: 清除所有大于等于该高度的数据
	RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error)
	
	// HistoryClear 用于统一清除历史数据，不需要请求直接返回即可
	// lteEpoch: 清除所有小于等于该高度数据
	HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error)
}

// Task 用于数据同步
// 特点：对高度的连续性没有依赖
type Task interface {
	base
	// Exec 执行任务
	// 将会检查任务的上次落库高度, 以同步任务高度最低的任务高度为起点，起到自动补全中间缺失数据的效果
	// 当高度落后超过配置的阈值时，则触发并发同步
	// 注意：任务中的数据库操作应该使用传入的 context.Context，其中加入了事务
	Exec(ctx *Context) (err error)
}

// TaskGroup 任务分组
// 一个任务分组，是一个串行执行单位
// 多个分组之间，将会并发执行
type TaskGroup []Task

// Calculator 连续性依赖 用于指标计算
// 特点：对高度有依赖，以全站最慢的同步任务高度为基准，确保同步数据的连续性
// Calculator 和 Task 在注册时，和数据库检查是否已执行时(sync_task_epochs 表) 无区别
type Calculator interface {
	base
	// Calc 执行计算任务
	Calc(ctx *Context) (err error)
}
