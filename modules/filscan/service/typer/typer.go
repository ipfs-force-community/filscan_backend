package typer

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

func NewTyper(repo repository.ChangeActorTask, adapter londobell.Adapter) *Typer {
	return &Typer{repo: repo, adapter: adapter}
}

type Typer struct {
	repo    repository.ChangeActorTask
	adapter londobell.Adapter
}

func (t *Typer) ActorType(str string) (actorType string, err error) {
	
	addr := chain.SmartAddress(str)
	if !addr.IsValid() {
		err = fmt.Errorf("invilad addr: %s", str)
		return
	}
	
	var actor *po.ActorPo
	if addr.IsID() {
		actor, err = t.repo.GetActorById(context.Background(), str)
	} else {
		actor, err = t.repo.GetActorByRobust(context.Background(), str)
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		actor = nil
		return
	}
	
	if actor != nil {
		actorType = actor.Type
		return
	}
	
	state, err := t.adapter.Actor(context.Background(), addr, nil)
	if err != nil {
		err = fmt.Errorf("address not found")
		return
	}
	
	actor = &po.ActorPo{
		Id:          state.ActorID,
		Robust:      nil,
		Type:        state.ActorType,
		Code:        state.Code.String(),
		CreatedTime: nil,
		LastTxTime:  nil,
		Balance:     state.Balance,
	}
	
	if !chain.SmartAddress(state.ActorAddr).IsEmpty() {
		actor.Robust = &state.ActorAddr
	}
	
	actorType = state.ActorType
	
	_ = t.repo.AddActors(context.Background(), []*po.ActorPo{actor})
	
	return
}

func (t *Typer) MinerSectorSize(miner string) (size int64, err error) {
	
	size, err = t.repo.GetMinerSizeOrZero(context.Background(), miner)
	if err != nil {
		return
	}
	if size > 0 {
		return
	}
	
	info, err := t.adapter.Miner(context.Background(), chain.SmartAddress(miner), nil)
	if err != nil {
		return
	}
	
	size = info.SectorSize
	
	return
}
