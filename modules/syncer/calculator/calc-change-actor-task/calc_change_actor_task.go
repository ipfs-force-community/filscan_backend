package calc_change_actor_task

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/gozelle/async/parallel"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"

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

	// 这是 actor 同步器里每个高度最重的一步。历史上出现过「每高度耗时远大于任务本身」的落后问题，
	// 这里把各阶段耗时打出来，便于一眼定位瓶颈（读余额 / 取父 tipset / 读老行 / 组装 / 写库）。
	tStart := time.Now()

	balances, err := c.repo.GetActorBalances(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}
	// 准备 actors
	if len(balances) == 0 {
		return
	}
	tBalances := time.Since(tStart)

	preTipset, err := ctx.Agg().ParentTipset(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}
	tParent := time.Since(tStart)

	// 先按变动列表取出库里已有的 Actor 行，用途有二：
	//  1) 判定本次是 New 还是 Update（原实现把它放在组装之后，且用 adapter 的两次状态查询做重复判断）；
	//  2) 复用其 created_time，避免每个 actor 再向聚合器查一次创建时间（原实现的主要开销来源）。
	var ids []string
	for _, v := range balances {
		ids = append(ids, v.ActorId)
	}
	sort.Strings(ids)

	oldMap := make(map[string]*po.ActorPo, len(ids))
	for i := 0; i < len(ids); i += 1000 {
		ml := i + 1000
		if ml > len(ids) {
			ml = len(ids)
		}
		var oldActors []*po.ActorPo
		oldActors, err = c.repo.GetActorsByIds(ctx.Context(), ids[i:ml])
		if err != nil {
			return
		}
		for _, v := range oldActors {
			oldMap[v.Id] = v
		}
	}
	tOld := time.Since(tStart)

	// 补全 Actor 信息
	actors, err := c.queryActors(ctx, balances, oldMap, chain.Epoch(preTipset[0].ID))
	if err != nil {
		return
	}
	tQuery := time.Since(tStart)

	// 按 actor id 排序后再写：多个 actor、且 20 个高度并发写同一批表时，
	// 统一的加锁顺序能显著减少 PostgreSQL 死锁（SQLSTATE 40P01）引发的整窗等待重试。
	sort.Slice(actors, func(i, j int) bool { return actors[i].Id < actors[j].Id })

	actorsIds, actions := buildActorActions(ctx.Epoch(), actors, oldMap)

	// 保存
	err = c.save(ctx.Context(), actorsIds, actors, actions)
	if err != nil {
		return
	}
	tSave := time.Since(tStart)

	ctx.Debugf("[耗时拆解] 变动=%d 读余额=%s 取父tipset=%s 读老行=%s 组装=%s 写库=%s 合计=%s",
		len(balances), tBalances, tParent-tBalances, tOld-tParent, tQuery-tOld, tSave-tQuery, tSave)

	return
}

// buildActorActions 生成 actor id 列表与动作记录（已在库中=Update，否则=New）。
// 输入 actors 需已按 Id 排序，保证写库时加锁顺序一致。
func buildActorActions(epoch chain.Epoch, actors []*po.ActorPo, oldMap map[string]*po.ActorPo) (ids []string, actions []*po.ActorAction) {
	for _, v := range actors {
		ids = append(ids, v.Id)
		var action int
		if _, ok := oldMap[v.Id]; ok {
			action = po.ActorActionUpdate
		} else {
			action = po.ActorActionNew
		}
		actions = append(actions, &po.ActorAction{
			Epoch:   epoch.Int64(),
			ActorId: v.Id,
			Action:  action,
		})
	}
	return
}

func (c CalcChangeActorTask) queryActors(ctx *syncer.Context, balances []*po.ActorBalance, oldMap map[string]*po.ActorPo, prevEpoch chain.Epoch) (actors []*po.ActorPo, err error) {

	var runners []parallel.Runner[*po.ActorPo]

	for _, v := range balances {
		balance := v
		runners = append(runners, func(_ context.Context) (*po.ActorPo, error) {
			return c.PrepareActor(ctx, chain.SmartAddress(balance.ActorId), prevEpoch, oldMap[balance.ActorId])
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

// PrepareActor 组装一条 actor 行。
// old 非空表示该 actor 已在库里（本次只是余额/状态更新）：此时不再做 detectNewComer 的两次状态查询
// （那两次只在判定「本高度新创建的 actor」时才有意义），也不再向聚合器查询创建时间，直接复用库里的值。
func (c CalcChangeActorTask) PrepareActor(ctx *syncer.Context, actorId chain.SmartAddress, preEpoch chain.Epoch, old *po.ActorPo) (r *po.ActorPo, err error) {

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

	if old != nil {
		r.CreatedTime = old.CreatedTime
	} else {
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
