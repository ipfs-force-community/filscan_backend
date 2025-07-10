package test

import (
	"context"
	"log"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewTask() *Task {
	return &Task{}
}

var _ syncer.Task = (*Task)(nil)

type Task struct {
}

func (t Task) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (t Task) Name() string {
	return "test-task"
}

func (t Task) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	log.Printf("同步触发回滚：>= %s", gteEpoch)
	return
}

func (t Task) Exec(ctx *syncer.Context) (err error) {
	log.Printf("执行同步: %s", ctx.Epoch())

	_, err = ctx.Datamap().Get(syncer.TracesTey)

	return
}

func NewCalc() *Calc {
	return &Calc{}
}

var _ syncer.Calculator = (*Calc)(nil)

type Calc struct {
}

func (c Calc) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (c Calc) Name() string {
	return "calc-test-task"
}

func (c Calc) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	log.Printf("计算器触发回滚：>= %s", gteEpoch)
	return
}

func (c Calc) Calc(ctx *syncer.Context) (err error) {
	log.Printf("执行计算: %s", ctx.Epoch())
	return
}
