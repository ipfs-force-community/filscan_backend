package evm_transaction

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"strings"
)

func NewEvmTransactionTask(dal *dal.EvmTransactionDal) *EvmTransactionTask {
	return &EvmTransactionTask{dal: dal}
}

var _ syncer.Task = (*EvmTransactionTask)(nil)

type EvmTransactionTask struct {
	dal *dal.EvmTransactionDal
}

func (e EvmTransactionTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (e EvmTransactionTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = e.dal.DeleteEvmTransactions(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (e EvmTransactionTask) Name() string {
	return "evm-transaction-task"
}

func (e EvmTransactionTask) Exec(ctx *syncer.Context) (err error) {

	if ctx.Empty() {
		return
	}

	val, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	traces := val.([]*londobell.TraceMessage)

	var currentEpoch chain.Epoch
	if len(traces) == 0 {
		ctx.Debugf("traces is empty")
		return
	} else {
		currentEpoch = chain.Epoch(traces[0].Epoch)
	}

	var evmTransaction []*po.EvmTransaction

	for _, trace := range traces {
		actorTypeSplit := strings.Split(trace.Detail.Actor, "/")
		if trace.Detail != nil && actorTypeSplit[len(actorTypeSplit)-1] == "evm" {
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
			evmTransaction = append(evmTransaction, &po.EvmTransaction{
				TraceID:      trace.ID,
				Epoch:        trace.Epoch,
				MessageCid:   cid,
				ActorID:      actor.ActorID,
				ActorAddress: actor.DelegatedAddr,
				UserAddress:  trace.From.Address(),
				GasCost:      gasCost,
				IsBlock:      trace.IsBlock,
				Value:        trace.Value,
				ExitCode:     exitCode,
				MethodName:   trace.Detail.Method,
			})
			if trace.IsBlock == true {
				err = e.dal.SaveEvmTransactionUser(ctx.Context(), &po.EvmTransactionUser{
					ActorID:       actor.ActorID,
					UserAddress:   trace.From.Address(),
					LatestTxEpoch: trace.Epoch,
				})
				if err != nil {
					return err
				}
			}
		}
	}

	if evmTransaction != nil {
		err = e.dal.SaveEvmTransactions(ctx.Context(), evmTransaction)
		if err != nil {
			return
		}
	}

	beforeEpoch := currentEpoch - 900
	err = e.dal.DeleteEvmTransactionsBeforeEpoch(ctx.Context(), beforeEpoch)
	if err != nil {
		return err
	}

	return
}
