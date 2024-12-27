package prosyncer

import (
	"context"
	"fmt"

	"github.com/gozelle/async/parallel"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

type ProCalculatorParams struct {
	Store              bool
	DisableSyncBalance bool
	DisableSyncFund    bool
	DisableSyncInfo    bool
}

func NewProCalculator(db *gorm.DB, params ProCalculatorParams) *ProCalculator {
	return &ProCalculator{
		minerInfos:    minerInfos{},
		minerFees:     minerFunds{},
		minerBalances: minerBalances{},
		saver:         newSaver(db),
		params:        params,
	}
}

var _ syncer.Calculator = (*ProCalculator)(nil)

type ProCalculator struct {
	minerInfos    minerInfos
	minerFees     minerFunds
	minerBalances minerBalances
	saver         *saver
	params        ProCalculatorParams
}

func (p ProCalculator) Name() string {
	return "pro-calculator"
}

func (p ProCalculator) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = p.saver.RollbackMinerFunds(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = p.saver.RollbackMinerInfos(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = p.saver.RollbackMinerBalances(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (p ProCalculator) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (p ProCalculator) Calc(ctx *syncer.Context) (err error) {

	// 限制整点触发
	if ctx.Epoch()%120 != 0 {
		return
	}
	epochs := chain.NewLCRORange(ctx.Epoch()-120, ctx.Epoch())

	miners, err := p.prepareMines(ctx, epochs)
	if err != nil {
		return
	}

	g := parallel.NewGroup()

	var (
		infos    []*propo.MinerInfo
		fees     []*propo.MinerFund
		balances []*propo.MinerBalance
	)

	g.Go(func() error {
		var e error

		if !p.params.DisableSyncBalance || !p.params.DisableSyncInfo {
			infos, e = p.minerInfos.syncMinerInfosFromAgg(ctx, epochs, miners)
			if e != nil {
				e = fmt.Errorf("prepare mienr infos error: %w", e)
				return e
			}
			ctx.Debugf("miner infos query done")
		} else {
			ctx.Debugf("忽略 Infos 同步")
		}

		if !p.params.DisableSyncBalance {
			balances, e = p.minerBalances.prepareMinerBalances(ctx, epochs.LtEnd, infos)
			if e != nil {
				e = fmt.Errorf("prepare mienr balances error: %w", e)
				return e
			}
			ctx.Debugf("miner balances query done")
		} else {
			ctx.Debugf("忽略余额同步")
		}

		return nil
	})

	g.Go(func() error {
		if !p.params.DisableSyncFund {
			var e error
			fees, e = p.minerFees.prepareMinerFees(ctx, epochs, miners)
			if e != nil {
				e = fmt.Errorf("prepare mienr fees error: %w", e)
				return e
			}
			ctx.Debugf("miner fees query done")
		} else {
			ctx.Debugf("忽略消耗同步")
		}
		return nil
	})

	err = g.Wait()
	if err != nil {
		return
	}

	err = p.save(ctx, fees, infos, balances)
	if err != nil {
		return
	}

	return
}

func (p ProCalculator) prepareMines(ctx *syncer.Context, epochs chain.LCRORange) (miners []*londobell.MinerInfo, err error) {

	lastHourMiners, err := ctx.Agg().MinersInfo(ctx.Context(), epochs.GteBegin, epochs.GteBegin.Next())
	if err != nil {
		return
	}
	currentHourMiners, err := ctx.Agg().MinersInfo(ctx.Context(), epochs.LtEnd, epochs.LtEnd.Next())
	if err != nil {
		return
	}
	mapping := map[string]*londobell.MinerInfo{}
	for _, v := range append(lastHourMiners, currentHourMiners...) {
		mapping[v.Miner.Address()] = v
	}
	for _, v := range mapping {
		miners = append(miners, v)
	}
	return
}

func (p ProCalculator) save(ctx *syncer.Context, fund []*propo.MinerFund, infos []*propo.MinerInfo, balances []*propo.MinerBalance) (err error) {
	if !p.params.Store {
		ctx.Infof("未保存")
		return
	}

	if !p.params.DisableSyncInfo {
		err = p.saver.SaveMinerInfos(ctx.Context(), infos)
		if err != nil {
			return
		}
	}

	if !p.params.DisableSyncFund {
		err = p.saver.SaveMinerFunds(ctx.Context(), fund)
		if err != nil {
			return
		}
	}

	if !p.params.DisableSyncBalance {
		err = p.saver.SaveMinerBalances(ctx.Context(), balances)
		if err != nil {
			return
		}
	}
	return
}
