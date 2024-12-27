package calc_estimate_miner_gas

import (
	"context"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewCalEstimateMinerGas(repo repository.SyncerTraceTaskRepo) *CalEstimateMinerGas {
	return &CalEstimateMinerGas{repo: repo}
}

var _ syncer.Calculator = (*CalEstimateMinerGas)(nil)

type CalEstimateMinerGas struct {
	repo repository.SyncerTraceTaskRepo
}

func (c CalEstimateMinerGas) Name() string {
	return "calc-estimate-miner-gas"
}

func (c CalEstimateMinerGas) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	// task 任务回滚即可
	return
}

func (c CalEstimateMinerGas) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	return
}

func (c CalEstimateMinerGas) Calc(ctx *syncer.Context) (err error) {
	
	if ctx.Empty() {
		return
	}
	
	items, err := c.repo.GetBaseGasCosts(ctx.Context(), ctx.Epoch()-120*8, ctx.Epoch())
	if err != nil {
		return
	}
	l := decimal.NewFromInt(int64(len(items)))
	if l.Equal(decimal.Zero) {
		return
	}
	
	baseFee := items[0].BaseGas
	
	limit32 := decimal.Decimal{}
	limit64 := decimal.Decimal{}
	count32 := int64(0)
	count64 := int64(0)
	for _, v := range items {
		if v.AvgGasLimit32.GreaterThan(decimal.Zero) {
			limit32 = limit32.Add(v.AvgGasLimit32)
			count32++
		}
		if v.AvgGasLimit64.GreaterThan(decimal.Zero) {
			limit64 = limit64.Add(v.AvgGasLimit64)
			count64++
		}
	}
	
	sectorFee32 := decimal.Decimal{}
	sectorFee64 := decimal.Decimal{}
	
	if count32 > 0 {
		sectorFee32 = baseFee.Mul(limit32.Div(decimal.NewFromInt(count32))).Mul(decimal.NewFromInt(1024 / 32))
	}
	if count64 > 0 {
		sectorFee64 = baseFee.Mul(limit64.Div(decimal.NewFromInt(count64))).Mul(decimal.NewFromInt(1024 / 64))
	}
	
	err = c.repo.UpdateBaseGasCostSectorGas(ctx.Context(), ctx.Epoch(), sectorFee32, sectorFee64)
	if err != nil {
		return
	}
	
	return
}
