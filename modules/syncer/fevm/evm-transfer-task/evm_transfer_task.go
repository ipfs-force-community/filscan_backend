package evm_transfer_task

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/biz/browser"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"strings"
)

func NewEVMTransferTask(repo repository.EvmTransferRepo) *EVMTransferTask {
	return &EVMTransferTask{repo: repo}
}

var _ syncer.Task = (*EVMTransferTask)(nil)

type EVMTransferTask struct {
	repo repository.EvmTransferRepo
}

func (e EVMTransferTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (e EVMTransferTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = e.repo.DeleteEvmTransfers(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = e.repo.DeleteEvmTransferStats(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (e EVMTransferTask) Name() string {
	return "evm-transfer-task"
}

func (e EVMTransferTask) Exec(ctx *syncer.Context) (err error) {

	ctx.Debugf("开始同步...")
	if ctx.Empty() {
		return
	}

	val, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	traces := val.([]*londobell.TraceMessage)

	if len(traces) == 0 {
		ctx.Debugf("traces is empty")
		return
	}

	var evmTransfers []*po.EvmTransfer
	var evmTransferStats1h []*po.EvmTransferStat

	for _, trace := range traces {
		actorTypeSplit := strings.Split(trace.Detail.Actor, "/")
		if trace.Detail != nil && actorTypeSplit[len(actorTypeSplit)-1] == "evm" && trace.Detail.Method == "InvokeContract" && trace.IsBlock == true {
			var actor *londobell.ActorState
			actor, err = ctx.Adapter().Actor(ctx.Context(), trace.To, nil)
			if err != nil {
				return
			}

			var gasCost decimal.Decimal
			if trace.GasCost != nil {
				gasCost = trace.GasCost.TotalCost
			}
			var exitCode *int
			if trace.MsgRct != nil {
				exitCode = &trace.MsgRct.ExitCode
			}
			var cid string
			if trace.SignedCid != nil && *trace.SignedCid != "" {
				cid = *trace.SignedCid
			} else {
				cid = trace.Cid
			}

			evmTransfers = append(evmTransfers, &po.EvmTransfer{
				Epoch:        trace.Epoch,
				MessageCid:   cid,
				ActorID:      actor.ActorID,
				ActorAddress: actor.DelegatedAddr,
				UserAddress:  trace.From.Address(),
				Balance:      actor.Balance,
				GasCost:      gasCost,
				Value:        trace.Value,
				ExitCode:     exitCode,
				MethodName:   trace.Detail.Method,
			})
		}
	}

	if len(traces) != 0 && traces[0].Epoch%120 == 0 {
		evmTransferStats1h, err = e.HandlerEvmTransferStats(ctx.Context(), chain.Epoch(traces[0].Epoch))
		if err != nil {
			return
		}
	}

	ctx.Debugf("开始保存了, evmTransfers: %d, evmTransferStats1h: %d", len(evmTransfers), len(evmTransferStats1h))

	if evmTransfers != nil {
		err = e.SaveEvmTransfers(ctx.Context(), evmTransfers)
		if err != nil {
			return
		}
	}

	if evmTransferStats1h != nil {
		err = e.SaveEvmTransferStats(ctx.Context(), evmTransferStats1h)
		if err != nil {
			return
		}
	}

	return
}

func (e EVMTransferTask) SaveEvmTransfers(ctx context.Context, evmActors []*po.EvmTransfer) (err error) {
	err = e.repo.SaveEvmTransfers(ctx, evmActors)
	if err != nil {
		return
	}
	return
}

func (e EVMTransferTask) HandlerEvmTransferStats(ctx context.Context, epoch chain.Epoch) (evmActorStats1h []*po.EvmTransferStat, err error) {
	// 处理 interval 变化
	evmActorStats1h, err = e.getAccEvmTransferStats(ctx, epoch, "1h")
	if err != nil {
		return
	}

	return
}

func (e EVMTransferTask) getAccEvmTransferStats(ctx context.Context, epoch chain.Epoch, interval string) (evmTransferStats []*po.EvmTransferStat, err error) {
	evmTransfers, err := e.repo.GetEvmTransferStats(ctx, epoch)
	if err != nil {
		return
	}
	for _, evm := range evmTransfers {
		var ethAddress string
		ethAddress, err = browser.TransferToETHAddress(evm.ActorAddress)
		if err != nil {
			return
		}
		evmTransferStats = append(evmTransferStats, &po.EvmTransferStat{
			Epoch:            epoch.Int64(),
			ActorID:          evm.ActorID,
			Interval:         interval,
			AccTransferCount: evm.AccTransferCount,
			AccUserCount:     evm.AccUserCount,
			AccGasCost:       evm.AccGasCost,
			ActorBalance:     evm.ActorBalance,
			ActorAddress:     evm.ActorAddress,
			ContractAddress:  ethAddress,
			ContractName:     evm.ContractName,
		})
	}

	return
}

func (e EVMTransferTask) SaveEvmTransferStats(ctx context.Context, transferStats []*po.EvmTransferStat) (err error) {

	err = e.repo.SaveEvmTransferStats(ctx, transferStats)
	if err != nil {
		return
	}

	return
}
