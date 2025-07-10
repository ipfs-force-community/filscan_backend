package calc_change_actor_task

import (
	"context"
	"github.com/gozelle/async/parallel"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"strings"
	
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewCalcChangeActorTask(repo repository.ChangeActorTask) *CalcChangeActorTask {
	r := &CalcChangeActorTask{repo: repo}
	return r
}

var _ syncer.Calculator = (*CalcChangeActorTask)(nil)

type CalcChangeActorTask struct {
	repo repository.ChangeActorTask
}

func (c CalcChangeActorTask) Name() string {
	return "calc-change-actor-task"
}

func (c CalcChangeActorTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	
	actions, err := c.repo.GetActorActionsAfterEpoch(ctx, gteEpoch)
	if err != nil {
		return
	}
	var deleteIds []string
	for _, v := range actions {
		if v.Action == po.ActorActionNew {
			deleteIds = append(deleteIds, v.ActorId)
		}
	}
	
	err = c.repo.DeleteActorsByIds(ctx, deleteIds)
	if err != nil {
		return
	}
	
	err = c.repo.DeleteActorActions(ctx, gteEpoch)
	if err != nil {
		return
	}
	
	return
}

func (c CalcChangeActorTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (c CalcChangeActorTask) save(ctx context.Context, actorIds []string, actors []*po.ActorPo, actions []*po.ActorAction) (err error) {
	err = c.repo.DeleteActorsByIds(ctx, actorIds)
	if err != nil {
		return
	}
	err = c.repo.AddActors(ctx, actors)
	if err != nil {
		return
	}
	err = c.repo.AddActorActions(ctx, actions)
	if err != nil {
		return
	}
	return
}

func (c CalcChangeActorTask) Calc(ctx *syncer.Context) (err error) {
	
	if ctx.Empty() {
		return
	}
	
	balances, err := c.repo.GetActorBalances(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}
	// 准备 actors
	if len(balances) == 0 {
		return
	}
	
	preTipset, err := ctx.Agg().ParentTipset(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}
	
	// 补全 Actor 信息
	actors, err := c.queryActors(ctx, balances, chain.Epoch(preTipset[0].ID))
	if err != nil {
		return
	}
	var actions []*po.ActorAction
	var actorsIds []string
	for _, v := range actors {
		actorsIds = append(actorsIds, v.Id)
	}
	oldMap := map[string]struct{}{}

	// 查询已有的 Actor

	for i := 0; i < len(actors); i+=1000 {
		ml := i+1000
		if ml > len(actors) {
			ml = len(actors)
		}
		oldActors, err := c.repo.GetActorsByIds(ctx.Context(), actorsIds[i:ml])
		if err != nil {
			return err
		}
		for _, v := range oldActors {
			oldMap[v.Id] = struct{}{}
		}
	}
	// 准备 Action

	for _, v := range actors {
		var action int
		if _, ok := oldMap[v.Id]; ok {
			action = po.ActorActionUpdate
		} else {
			action = po.ActorActionNew
		}
		actions = append(actions, &po.ActorAction{
			Epoch:   ctx.Epoch().Int64(),
			ActorId: v.Id,
			Action:  action,
		})
	}
	
	// 保存
	err = c.save(ctx.Context(), actorsIds, actors, actions)
	if err != nil {
		return
	}
	
	return
}

func (c CalcChangeActorTask) queryActors(ctx *syncer.Context, balances []*po.ActorBalance, prevEpoch chain.Epoch) (actors []*po.ActorPo, err error) {
	
	var runners []parallel.Runner[*po.ActorPo]
	
	for _, v := range balances {
		balance := v
		runners = append(runners, func(_ context.Context) (*po.ActorPo, error) {
			return c.PrepareActor(ctx, chain.SmartAddress(balance.ActorId), prevEpoch)
		})
	}
	
	ch := parallel.Run[*po.ActorPo](ctx.Context(), 10, runners)
	
	err = parallel.Wait[*po.ActorPo](ch, func(v *po.ActorPo) error {
		actors = append(actors, v)
		return nil
	})
	if err != nil {
		return
	}
	
	return
}

func (c CalcChangeActorTask) PrepareActor(ctx *syncer.Context, actorId chain.SmartAddress, preEpoch chain.Epoch) (r *po.ActorPo, err error) {
	
	epoch := ctx.Epoch()
	actorState, err := ctx.Adapter().Actor(ctx.Context(), actorId, &epoch)
	if err != nil {
		return
	}
	
	r = &po.ActorPo{
		Id:          actorState.ActorID,
		Robust:      nil,
		Type:        actorState.ActorType,
		Code:        actorState.Code.String(),
		CreatedTime: nil,
		LastTxTime:  nil,
		Balance:     actorState.Balance,
	}
	
	if !chain.SmartAddress(actorState.ActorAddr).IsEmpty() {
		r.Robust = &actorState.ActorAddr
	}
	
	ok, e := c.detectNewComer(ctx.Adapter(), ctx.Epoch(), actorId)
	if e != nil {
		return nil, e
	}
	var createdAtEpoch chain.Epoch
	if ok {
		createdAtEpoch = preEpoch
	} else {
		createdAtEpoch, err = ctx.Agg().CreateTime(ctx.Context(), actorId)
		if err != nil {
			return
		}
	}
	if createdAtEpoch > 0 {
		t := createdAtEpoch.Time()
		r.CreatedTime = &t
	}
	
	// 最新交易时间记录上一个出块的时间
	lastTxTime := preEpoch.Time()
	r.LastTxTime = &lastTxTime
	
	return
}

func (c CalcChangeActorTask) detectNewComer(adapter londobell.Adapter, epoch chain.Epoch, addr chain.SmartAddress) (ok bool, err error) {
	
	_, err = adapter.Actor(context.Background(), addr, &epoch)
	if err != nil {
		if strings.Contains(err.Error(), "actor not found") {
			err = nil
		}
		return
	}
	
	pre := epoch - 1
	_, err = adapter.Actor(context.Background(), addr, &pre)
	if err != nil {
		if strings.Contains(err.Error(), "actor not found") {
			err = nil
			ok = true
		}
		return
	}
	
	return
}
