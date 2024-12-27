package builtin_actor_task

import (
	"context"
	"encoding/json"

	"github.com/filecoin-project/go-state-types/builtin"
	logging "github.com/gozelle/logger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewBaselineTask(repo repository.BaselineTaskRepo) *BaselineTask {
	return &BaselineTask{repo: repo, log: logging.NewLogger("baseline-task")}
}

var _ syncer.Task = (*BaselineTask)(nil)

type BaselineTask struct {
	repo repository.BaselineTaskRepo
	log  *logging.Logger
}

func (b BaselineTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (b BaselineTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = b.repo.DeleteBuiltActorStates(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (b BaselineTask) Name() string {
	return "baseline-task"
}

func (b BaselineTask) Exec(ctx *syncer.Context) (err error) {

	if ctx.Empty() {
		return
	}

	rewardActor, rewardState, err := b.getRewardActorState(ctx.Context(), ctx.Adapter(), ctx.Epoch())
	if err != nil {
		return
	}

	powerActor, powerState, err := b.getPowerActorState(ctx.Context(), ctx.Adapter(), ctx.Epoch())
	if err != nil {
		return
	}

	d, err := json.Marshal(rewardState)
	if err != nil {
		return
	}
	rewardPo := &po.BuiltinActorStatePo{
		Epoch:   ctx.Epoch().Int64(),
		Actor:   builtin.RewardActorAddr.String(),
		Balance: rewardActor.Balance,
		State:   string(d),
	}

	d, err = json.Marshal(powerState)
	if err != nil {
		return
	}
	powerPo := &po.BuiltinActorStatePo{
		Epoch:   ctx.Epoch().Int64(),
		Actor:   builtin.StoragePowerActorAddr.String(),
		Balance: powerActor.Balance,
		State:   string(d),
	}

	err = b.save(ctx.Context(), rewardPo, powerPo)
	if err != nil {
		return
	}

	return
}

func (b BaselineTask) getRewardActorState(ctx context.Context, adapter londobell.Adapter, epoch chain.Epoch) (actor *londobell.ActorState, state *londobell.RewardActorState, err error) {

	actor, err = adapter.Actor(ctx, chain.SmartAddress(builtin.RewardActorAddr.String()), &epoch)
	if err != nil {
		return
	}

	state = new(londobell.RewardActorState)
	d, err := json.Marshal(actor.State)
	if err != nil {
		return
	}
	err = json.Unmarshal(d, state)
	if err != nil {
		return
	}

	return
}

func (b BaselineTask) getPowerActorState(ctx context.Context, adapter londobell.Adapter, epoch chain.Epoch) (actor *londobell.ActorState, state *londobell.PowerActorState, err error) {

	actor, err = adapter.Actor(ctx, chain.SmartAddress(builtin.StoragePowerActorAddr.String()), &epoch)
	if err != nil {
		return
	}

	state = new(londobell.PowerActorState)
	d, err := json.Marshal(actor.State)
	if err != nil {
		return
	}
	err = json.Unmarshal(d, state)
	if err != nil {
		return
	}

	return
}

func (b BaselineTask) save(ctx context.Context, rewardPo, powerPo *po.BuiltinActorStatePo) (err error) {

	err = b.repo.SaveBuiltActorStates(ctx, rewardPo, powerPo)
	if err != nil {
		return
	}

	return
}
