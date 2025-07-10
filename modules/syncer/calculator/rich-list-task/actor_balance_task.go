package rich_list_task

import (
	"context"
	
	"github.com/gozelle/mix"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/gorm"
)

func NewActorBalanceTask(db *gorm.DB, actorBalanceRepo repository.ActorBalanceTaskRepo) *ActorBalanceTask {
	return &ActorBalanceTask{db: db, actorBalanceRepo: actorBalanceRepo}
}

var _ syncer.Calculator = (*ActorBalanceTask)(nil)

type ActorBalanceTask struct {
	db               *gorm.DB
	actorBalanceRepo repository.ActorBalanceTaskRepo
}

func (a ActorBalanceTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (a ActorBalanceTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = a.actorBalanceRepo.DeleteActorsBalance(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (a ActorBalanceTask) Name() string {
	return "rich-list-task"
}

func (a ActorBalanceTask) Calc(ctx *syncer.Context) (err error) {
	
	if ctx.Empty() {
		return
	}
	
	currentDay := chain.CurrentEpoch().CurrentDay()
	if currentDay == ctx.Epoch() {
		filters := types.Filters{
			Index: 0,
			Limit: 1000,
			Start: &currentDay,
		}
		var richList *londobell.RichList
		richList, err = a.getRichList(ctx, filters)
		if err != nil {
			return err
		}
		if richList == nil || richList.TotalCount == 0 {
			err = mix.Warnf("获取富豪榜为空")
			return
		}
		var actorBalanceList []*actor.RichActor
		actorBalanceList, err = a.toActorsBalance(richList, currentDay)
		if err != nil {
			return err
		}
		err = a.save(ctx.Context(), actorBalanceList)
		if err != nil {
			return err
		}
	}
	return
}

func (a ActorBalanceTask) getRichList(ctx *syncer.Context, filters types.Filters) (actorState *londobell.RichList, err error) {
	actorState, err = ctx.Agg().RichList(ctx.Context(), filters)
	if err != nil {
		return
	}
	if actorState == nil {
		return
	}
	return
}

func (a ActorBalanceTask) toActorsBalance(source *londobell.RichList, epoch chain.Epoch) (target []*actor.RichActor, err error) {
	for _, rich := range source.RichList {
		actorBalance := &actor.RichActor{
			Epoch:     epoch.Int64(),
			ActorID:   rich.Addr.Address(),
			ActorType: rich.Code,
			Balance:   rich.Balance,
		}
		target = append(target, actorBalance)
	}
	return
}

func (a ActorBalanceTask) save(ctx context.Context, actorsBalance []*actor.RichActor) (err error) {
	err = a.actorBalanceRepo.SaveActorsBalance(ctx, actorsBalance)
	if err != nil {
		return
	}
	return
}
