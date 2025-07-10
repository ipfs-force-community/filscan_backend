package deal_proposal_task

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/filecoin-project/go-state-types/builtin"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	
	"github.com/filecoin-project/go-state-types/builtin/v8/market"
	
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
)

var _ syncer.Task = (*DealProposalTask)(nil)

type DealProposalTask struct {
	repo repository.DealProposalTaskRepo
}

func (dp DealProposalTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (dp DealProposalTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = dp.repo.DeleteDealProposals(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (dp DealProposalTask) Name() string {
	return "deal-proposal-task"
}

func NewDealProposalTask(repo repository.DealProposalTaskRepo) *DealProposalTask {
	return &DealProposalTask{repo: repo}
}

func (dp DealProposalTask) Exec(ctx *syncer.Context) (err error) {
	if ctx.Empty() {
		return
	}
	
	items, err := dp.exec(ctx)
	if err != nil {
		return
	}
	
	err = dp.save(ctx.Context(), items)
	if err != nil {
		return
	}
	
	return
}

func (dp DealProposalTask) save(ctx context.Context, items []*po.DealProposalPo) (err error) {
	err = dp.repo.SaveDealProposals(ctx, items...)
	if err != nil {
		return
	}
	return
}

func (dp DealProposalTask) exec(ctx *syncer.Context) (items []*po.DealProposalPo, err error) {
	
	r, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	
	traces := r.([]*londobell.TraceMessage)
	
	if len(traces) == 0 {
		err = fmt.Errorf("traces is emtpy")
		return
	}
	
	for i := range traces {
		
		if traces[i].MsgRct == nil {
			continue
		}
		
		if traces[i].MsgRct.ExitCode != 0 {
			continue
		}
		
		if traces[i].Msg.To.Address() != builtin.StorageMarketActorAddr.String() || traces[i].Msg.Method != 4 {
			continue
		}
		
		returns := market.PublishStorageDealsReturn{}
		if v, ok := traces[i].Return.(string); ok {
			err = json.Unmarshal([]byte(v), &returns)
			if err != nil {
				err = fmt.Errorf("marshal publish storage deal failed")
				return
			}
			
			for j := 0; j < len(returns.IDs); j++ {
				cid := traces[i].Cid
				if traces[i].SignedCid != nil && *traces[i].SignedCid != "" {
					cid = *traces[i].SignedCid
				}
				items = append(items, &po.DealProposalPo{
					Epoch:  ctx.Epoch().Int64(),
					DealID: uint64(returns.IDs[j]),
					Cid:    cid,
				})
			}
		}
	}
	
	return
}
