package trace_task

import (
	"context"
	"fmt"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/typer"
	"gorm.io/gorm"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewTraceTask(db *gorm.DB, adapter londobell.Adapter) *Trace {
	return &Trace{
		repo:                   dal.NewSyncerTraceTaskDal(db),
		MinerGasCostCalculator: NewMinerGasCostCalculator(typer.NewTyper(dal.NewChangeActorTaskDal(db), adapter)),
	}
}

var _ syncer.Task = (*Trace)(nil)

type Trace struct {
	MinerGasCostCalculator *MinerGasCostCalculator
	MessageGasCost         MethodGasFee
	repo                   repository.SyncerTraceTaskRepo
}

func (t Trace) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (t Trace) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = t.repo.DeleteBaseGasCosts(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = t.repo.DeleteMethodGasFees(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = t.repo.DeleteMinerGasFees(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (t Trace) save(ctx context.Context, statBaseGas *stat.BaseGasCost, messageGas []*po.MethodGasFee, minerGasFees []*po.MinerGasFee) (err error) {

	lastBaseGasCost, err := t.repo.GetLastBaseGasCostOrNil(ctx, statBaseGas.Epoch)
	if err != nil {
		return
	}
	statBaseGas.AccMessages = statBaseGas.Messages
	if lastBaseGasCost != nil {
		statBaseGas.AccMessages += lastBaseGasCost.AccMessages
	}

	// 保存基础手续费
	err = t.repo.SaveBaseGasCost(ctx, statBaseGas)
	if err != nil {
		return
	}

	// 保存消息手续费
	err = t.repo.SaveMethodGasFees(ctx, messageGas)
	if err != nil {
		return
	}

	// 保存 Miner Gas 消耗信息
	err = t.repo.SaveMinerGasFees(ctx, minerGasFees)
	if err != nil {
		return
	}

	return
}

func (t Trace) Name() string {
	return "trace-task"
}

func (t Trace) Exec(ctx *syncer.Context) (err error) {

	if ctx.Empty() {
		return
	}

	val, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	traces := val.([]*londobell.TraceMessage)

	if len(traces) == 0 {
		return fmt.Errorf("traces is emtpy")
	}

	cctx := &TraceContext{
		Context: ctx,
		traces:  traces,
	}

	// 同步 MinerGas
	err = t.syncMinerGas(cctx)
	if err != nil {
		return
	}

	// 同步基础手续费及 32G/64G Gas消耗
	err = t.syncBaseMinerFee(cctx)
	if err != nil {
		return
	}

	// 同步消息手续费
	err = t.MessageGasCost.parseMessageGas(cctx)
	if err != nil {
		return
	}

	// 检查新创建账号

	err = t.save(ctx.Context(), cctx.statBaseGas, cctx.methodGas, cctx.minerGas)
	if err != nil {
		return
	}

	return
}

func (t Trace) syncBaseMinerFee(ctx *TraceContext) (err error) {

	epoch := ctx.Epoch()
	tipset, err := ctx.Adapter().Epoch(ctx.Context.Context(), &epoch)
	if err != nil {
		return
	}

	fee32, fee64, limit32, limit64, err := t.MinerGasCostCalculator.Calc(ctx.Context, ctx.minerGas)
	if err != nil {
		return
	}

	ctx.statBaseGas = &stat.BaseGasCost{
		Epoch:         ctx.Epoch(),
		BaseGas:       chain.AttoFil(tipset.BaseFee),
		SectorGas32:   fee32.Fee(),
		SectorGas64:   fee64.Fee(),
		Messages:      int64(len(ctx.traces)),
		AccMessages:   0,
		AvgGasLimit32: limit32,
		AvgGasLimit64: limit64,
		SectorFee32:   decimal.Decimal{},
		SectorFee64:   decimal.Decimal{},
	}

	return
}

func (t Trace) syncMinerGas(ctx *TraceContext) (err error) {

	pre, err := ctx.Agg().AggPreNetFee(ctx.Context.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}
	preMap := map[string]decimal.Decimal{}
	for _, v := range pre { //todo 这里数据也会有点问题
		preMap[v.Miner.Address()] = v.AggFee
	}

	prove, err := ctx.Agg().AggProNetFee(ctx.Context.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}
	proveMap := map[string]decimal.Decimal{}
	for _, v := range prove { //todo 这里数据也会有点问题
		proveMap[v.Miner.Address()] = v.AggFee
	}

	sectorGas, err := ctx.Agg().MinerGasCost(ctx.Context.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}
	sectorGasMap := map[string]decimal.Decimal{}
	for _, v := range sectorGas {
		sectorGasMap[v.ID.Address()] = v.GasCost
	}

	wdPostMap, err := t.prepareWdPostGasMap(ctx)
	if err != nil {
		return
	}

	minerGasMap := map[string]*po.MinerGasFee{}

	for k, v := range preMap {
		if _, ok := minerGasMap[k]; !ok {
			minerGasMap[k] = &po.MinerGasFee{}
		}
		minerGasMap[k].PreAgg = v
	}

	for k, v := range proveMap {
		if _, ok := minerGasMap[k]; !ok {
			minerGasMap[k] = &po.MinerGasFee{}
		}
		minerGasMap[k].ProveAgg = v
	}

	for k, v := range sectorGasMap {
		if _, ok := minerGasMap[k]; !ok {
			minerGasMap[k] = &po.MinerGasFee{}
		}
		minerGasMap[k].SectorGas = v
	}

	for k, v := range wdPostMap {
		if _, ok := minerGasMap[k]; !ok {
			minerGasMap[k] = &po.MinerGasFee{}
		}
		minerGasMap[k].WdPostGas = v
	}

	for k, v := range minerGasMap {
		v.Miner = k
		v.Epoch = ctx.Epoch().Int64()
		v.SealGas = v.PreAgg.Add(v.ProveAgg).Add(v.SectorGas)
		ctx.minerGas = append(ctx.minerGas, v)
	}

	return
}

func (t Trace) prepareWdPostGasMap(ctx *TraceContext) (r map[string]decimal.Decimal, err error) {

	r = map[string]decimal.Decimal{}

	for _, v := range ctx.traces {
		if v.Detail != nil && v.Detail.Method == "SubmitWindowedPoSt" && v.GasCost != nil {
			if _, ok := r[v.To.Address()]; !ok {
				r[v.To.Address()] = decimal.Decimal{}
			}
			r[v.To.Address()] = r[v.To.Address()].Add(v.GasCost.TotalCost)
		}
	}

	return
}
