package prosyncer

import (
	"fmt"
	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type minerInfos struct {
}

func (m minerInfos) syncMinerInfosFromAgg(ctx *syncer.Context, epochs chain.LCRORange, miners []*londobell.MinerInfo) (infos []*propo.MinerInfo, err error) {
	
	r, err := ctx.Agg().MinersInfo(ctx.Context(), epochs.LtEnd, epochs.LtEnd.Next())
	if err != nil {
		return
	}
	
	ctx.Infof("高度: %s Miners infos: %d", epochs.LtEnd, len(r))
	if len(r) == 0 {
		err = fmt.Errorf("agg miner infos 无数据")
		return
	}
	
	mapping := map[string]struct{}{}
	for _, v := range r {
		var controllers []string
		for _, vv := range v.ControlAddresses {
			controllers = append(controllers, vv.Address())
		}
		info := &propo.MinerInfo{
			Epoch:           epochs.LtEnd.Int64(),
			Miner:           v.Miner.Address(),
			Owner:           v.Owner.Address(),
			Worker:          v.Worker.Address(),
			Controllers:     controllers,
			Beneficiary:     v.Beneficiary.Address(),
			RawBytePower:    v.RawBytePower,
			QualityAdjPower: v.QualityAdjPower,
			Pledge:          v.InitialPledge,
			LiveSectors:     v.LiveSectorSector,
			ActiveSectors:   v.ActiveSectorCount,
			FaultSectors:    v.FaultSectorCount,
			SectorSize:      v.SectorSize,
			Padding:         false,
		}
		infos = append(infos, info)
		mapping[info.Miner] = struct{}{}
	}
	
	// 判断上一个小时还有算力，当前无算力的情况
	// 全部给 0 值
	for _, v := range miners {
		if _, ok := mapping[v.Miner.Address()]; !ok {
			var controllers []string
			for _, vv := range v.ControlAddresses {
				controllers = append(controllers, vv.Address())
			}
			infos = append(infos, &propo.MinerInfo{
				Epoch:           epochs.LtEnd.Int64(),
				Miner:           v.Miner.Address(),
				Owner:           v.Owner.Address(),
				Worker:          v.Worker.Address(),
				Controllers:     controllers,
				Beneficiary:     v.Beneficiary.Address(),
				RawBytePower:    decimal.Decimal{},
				QualityAdjPower: decimal.Decimal{},
				Pledge:          decimal.Decimal{},
				LiveSectors:     0,
				ActiveSectors:   0,
				FaultSectors:    0,
				SectorSize:      0,
				Padding:         true,
			})
		}
	}
	
	return
}
