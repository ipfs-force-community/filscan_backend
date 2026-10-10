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
	
	// currentDay = 今天（本机时钟，北京时间）的起始高度。
	// miner 系子同步器按设计落后链头约 1000 高度（≈8 小时），游标会「越过」而非「恰好等于」
	// 当天边界（旧写法 `currentDay == ctx.Epoch()` 因此永远不成立、从不写入，已核实）。
	// 故判据改为「游标已走到或越过当天边界」。
	currentDay := chain.CurrentEpoch().CurrentDay()
	if ctx.Epoch() < currentDay {
		// 游标还没到当天边界，等它追上。
		return
	}
	
	// 幂等：越过边界后，当天剩余的所有高度都会重复进到这里，而 chain.rich_actors 在
	// (epoch, actor_id) 上有唯一索引、重复写会硬报错。因此写入前必须回库确认「今天这一期
	// 还没写过」——不能只用内存标记（进程重启 / 离线回放 / 回滚后重进都会绕过内存态）。
	// 回滚会删掉冲突高度起的行，届时这里重新查到「不存在」并补写，正是期望的补写行为。
	exists, err := a.actorBalanceRepo.ExistsActorsBalance(ctx.Context(), currentDay)
	if err != nil {
		return err
	}
	if exists {
		return
	}
	
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
