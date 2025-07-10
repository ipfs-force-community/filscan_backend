package trace_task

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"sync"
)

type TraceContext struct {
	*syncer.Context
	actors      *TraceActors
	traces      []*londobell.TraceMessage
	methodGas   []*po.MethodGasFee
	minerGas    []*po.MinerGasFee
	statBaseGas *stat.BaseGasCost
}

type TraceActors struct {
	lock  sync.Mutex
	check map[actor.Id]*actor.Actor
}

func (t *TraceActors) AddActor(a *actor.Actor) {
	t.lock.Lock()
	defer func() {
		t.lock.Unlock()
	}()
	if t.check == nil {
		t.check = map[actor.Id]*actor.Actor{}
	}
	if _, ok := t.check[a.Id]; ok {
		return
	}
	t.check[a.Id] = a
}

func (t *TraceActors) GetActor(id actor.Id) *actor.Actor {
	if t.check == nil {
		return nil
	}
	return t.check[id]
}

func NewFee() *Fee {
	return &Fee{}
}

type Fee struct {
	lock sync.Mutex
	fee  chain.AttoFil
}

func (f *Fee) Fee() chain.AttoFil {
	return f.fee
}

func (f *Fee) AddFee(v decimal.Decimal) {
	f.lock.Lock()
	defer func() {
		f.lock.Unlock()
	}()
	f.fee = chain.AttoFil(f.fee.Decimal().Add(v))
}
