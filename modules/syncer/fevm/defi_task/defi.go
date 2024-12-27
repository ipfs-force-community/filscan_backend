package defi_task

import (
	"context"
	"fmt"
	"github.com/gozelle/logger"
	"sync"
	
	//"github.com/shopspring/decimal"
	
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
)

var log = logger.NewLogger("defi")

type DefiDashboardTask struct {
	node       *lotus_api.Node
	repo       repository.DefiRepo
	abiDecoder filscan.ABIDecoderAPI
	lastHeight int64
	mutex      sync.Mutex
}

func (e *DefiDashboardTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = e.repo.CleanDefiItems(ctx, gteEpoch)
	if err != nil {
		return err
	}
	e.mutex.Lock()
	defer e.mutex.Unlock()
	max, err := e.repo.GetMaxHeight(ctx)
	if err != nil {
		return err
	}
	if max < e.lastHeight {
		e.lastHeight = max
	}
	return
}

func (e *DefiDashboardTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	return nil
}

type SubTaskRes struct {
	Error error
	DD    po.DefiDashboard
}

func (e *DefiDashboardTask) Exec(ctx *syncer.Context) (err error) {
	if ctx.Empty() {
		return
	}
	
	r, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	
	traces := r.([]*londobell.TraceMessage)
	
	e.mutex.Lock()
	defer e.mutex.Unlock()
	
	if e.lastHeight+120 > traces[0].Epoch {
		return nil
	}
	
	end := e.lastHeight + 120
	
	ch := make(chan SubTaskRes, len(subTask))
	log.Info("start to deal subtask")
	wg := sync.WaitGroup{}
	for i := range subTask {
		wg.Add(1)
		task := subTask[i]
		go func() {
			defer wg.Done()
			tvl, err := task.GetTvl()
			if err != nil {
				ch <- SubTaskRes{
					Error: err,
				}
				return
			}
			
			users, err := task.GetUsers(e.repo)
			if err != nil {
				ch <- SubTaskRes{
					Error: err,
				}
				return
			}
			
			ch <- SubTaskRes{
				DD: po.DefiDashboard{
					Epoch:      int(end),
					Protocol:   task.GetProtocolName(),
					ContractId: task.GetContractId(),
					Tvl:        tvl.Usd,
					TvlInFil:   tvl.Fil,
					Users:      users,
					Url:        task.GetIconUrl(),
				},
			}
		}()
	}
	
	wg.Wait()
	close(ch)
	res := []*po.DefiDashboard{}
	for i := range ch {
		if i.Error != nil {
			log.Errorf("meet error in sub task %w", i.Error)
			return i.Error
		}
		r := i
		res = append(res, &r.DD)
	}
	err = e.repo.BatchSaveDefiItems(ctx.Context(), res)
	if err != nil {
		return err
	}
	e.lastHeight = end
	return
}

func NewDefiDashboardTask(abiDecoder filscan.ABIDecoderAPI, repo repository.DefiRepo, node *lotus_api.Node) *DefiDashboardTask {
	max, err := repo.GetMaxHeight(context.TODO())
	if err != nil {
		max = 0
	}
	fmt.Println(max)
	if max != 0 {
		return &DefiDashboardTask{
			node:       node,
			repo:       repo,
			abiDecoder: abiDecoder,
			lastHeight: max,
		}
	}
	ts, err := node.ChainHead(context.TODO())
	if err != nil {
		panic(err)
	}
	
	max = int64(ts.Height())
	return &DefiDashboardTask{
		node:       node,
		repo:       repo,
		abiDecoder: abiDecoder,
		lastHeight: max,
	}
}

func (e *DefiDashboardTask) Name() string {
	return "defi-dashboard-task"
}

var _ syncer.Task = (*DefiDashboardTask)(nil)
