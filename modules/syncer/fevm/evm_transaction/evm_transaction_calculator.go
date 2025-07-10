package evm_transaction

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/biz/browser"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewEvmContractCalculator(dal *dal.EvmTransactionDal) *EvmContractCalculator {
	return &EvmContractCalculator{
		dal: dal,
	}
}

var _ syncer.Calculator = (*EvmContractCalculator)(nil)

type EvmContractCalculator struct {
	dal *dal.EvmTransactionDal
}

func (e EvmContractCalculator) Name() string {
	return "evm-transaction-calculator"
}

func (e EvmContractCalculator) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = e.dal.DeleteEvmTransactions(ctx, gteEpoch)
	if err != nil {
		return
	}

	return
}

func (e EvmContractCalculator) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (e EvmContractCalculator) Calc(ctx *syncer.Context) (err error) {
	start := chain.Epoch(message_detail.HYGGE)
	if ctx.Epoch() > start && (ctx.Epoch()-start)%900 == 0 {
		var transactionStats []*po.EvmTransactionStat
		adapter := ctx.Adapter()
		transactionStats, err = e.getAccEvmTransactionStats(ctx.Context(), adapter, ctx.Epoch())
		if err != nil {
			return
		}
		err = e.SaveEvmTransactionStats(ctx.Context(), transactionStats)
		if err != nil {
			return
		}
	}

	return
}

func (e EvmContractCalculator) getAccEvmTransactionStats(ctx context.Context, adapter londobell.Adapter, epoch chain.Epoch) (transferStats []*po.EvmTransactionStat, err error) {
	evmTransfers, err := e.dal.GetEvmTransactionStats(ctx)
	if err != nil {
		return
	}
	for _, evm := range evmTransfers {
		var ethAddress string
		ethAddress, err = browser.TransferToETHAddress(evm.ActorAddress)
		if err != nil {
			return
		}
		transferStats = append(transferStats, &po.EvmTransactionStat{
			Epoch:               epoch.Int64(),
			ActorID:             evm.ActorID,
			AccTransactionCount: evm.AccTransactionCount,
			AccInternalTxCount:  evm.AccInternalTxCount,
			AccUserCount:        evm.AccUserCount,
			AccGasCost:          evm.AccGasCost,
			ActorAddress:        evm.ActorAddress,
			ContractAddress:     ethAddress,
			ContractName:        evm.ContractName,
		})
	}

	return
}

func (e EvmContractCalculator) SaveEvmTransactionStats(ctx context.Context, transactionStats []*po.EvmTransactionStat) (err error) {

	err = e.dal.SaveEvmTransactionStats(ctx, transactionStats)
	if err != nil {
		return
	}

	return
}
