package change_actor_task

import (
	"context"
	"fmt"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewChangeActorTask(repo repository.ChangeActorTask) *ChangeActorTask {
	return &ChangeActorTask{repo: repo}
}

var _ syncer.Task = (*ChangeActorTask)(nil)

type ChangeActorTask struct {
	repo repository.ChangeActorTask
}

func (c ChangeActorTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (c ChangeActorTask) Name() string {
	return "change-actor-task"
}

func (c ChangeActorTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = c.repo.DeleteActorBalances(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (c ChangeActorTask) Exec(ctx *syncer.Context) (err error) {
	now := time.Now()
	res, err := ctx.Adapter().ChangeActors(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}
	ctx.Debugf("请求 ChangeActors 完成，变动数: %d 耗时: %s", len(res), time.Since(now))

	lastTipset, err := ctx.Adapter().LastEpoch(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}

	// 这里使用上一个高度，是使用打包消息的高度，而不是出 receipt 的高度，与 agg 的获取创建时间保持统一
	prevEpoch := chain.Epoch(lastTipset.Epoch)

	// 准备 actors
	if len(res) == 0 {
		return
	}
	var actors []*po.ActorPo
	lxt := prevEpoch.Time()
	for k, v := range res {
		if !chain.SmartAddress(k).IsID() {
			err = fmt.Errorf("%s is not ID", k)
			return
		}
		actors = append(actors, &po.ActorPo{
			Id:         k,
			LastTxTime: &lxt,
			Balance:    v.Balance,
		})
	}

	// 准备保存
	err = c.save(ctx, actors, prevEpoch)
	if err != nil {
		return
	}

	return
}

func (c ChangeActorTask) save(ctx *syncer.Context, actors []*po.ActorPo, prevEpoch chain.Epoch) (err error) {

	for j := 0; j < len(actors); j += 1000 {
		var ids []string
		for i := j; i < len(actors) && i < j+1000; i++ {
			ids = append(ids, actors[i].Id)
		}

		exists, err := c.repo.GetExistsActors(ctx.Context(), ids)
		if err != nil {
			return err
		}

		// 准备余额变动
		var balances []*po.ActorBalance
		for i := j; i < len(actors) && i < j+1000; i++ {
			b := &po.ActorBalance{
				Epoch:     ctx.Epoch().Int64(),
				ActorId:   actors[i].Id,
				ActorType: nil,
				Balance:   actors[i].Balance,
				PrevEpoch: prevEpoch.Int64(),
			}
			// 更新的 Actor, 回填 Type
			if vv, ok := exists[b.ActorId]; ok {
				b.ActorType = &vv
			}
			balances = append(balances, b)
		}

		err = c.repo.AddActorBalances(ctx.Context(), balances)
		if err != nil {
			return err
		}

		ctx.Debugf("保存 balances: %d 耗时: %s", len(balances), ctx.SinceStart())
	}
	return
}
