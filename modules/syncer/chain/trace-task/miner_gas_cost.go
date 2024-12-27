package trace_task

import (
	"fmt"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/typer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

func NewMinerGasCostCalculator(typer *typer.Typer) *MinerGasCostCalculator {
	return &MinerGasCostCalculator{typer: typer}
}

type MinerGasCostCalculator struct {
	typer *typer.Typer
}

func (m MinerGasCostCalculator) Calc(ctx *syncer.Context, minerGases []*po.MinerGasFee) (fee32, fee64 *Fee, limit32, limit64 decimal.Decimal, err error) {
	
	fee32 = NewFee()
	fee64 = NewFee()
	
	const (
		sector32 = "32"
		sector64 = "64"
	)
	
	minerType := map[string]string{}
	for _, t := range minerGases {
		var sectorSize int64
		// 此处只为了获取扇区类型
		sectorSize, err = m.typer.MinerSectorSize(t.Miner)
		if err != nil {
			return
		}
		switch sectorSize {
		case 34359738368:
			fee32.AddFee(t.SealGas)
			minerType[t.Miner] = sector32
		case 68719476736:
			fee64.AddFee(t.SealGas)
			minerType[t.Miner] = sector64
		default:
			err = fmt.Errorf("unkonwn sector size: %d", sectorSize)
			return
		}
	}
	
	r, err := ctx.Datamap().Get(syncer.TracesTey)
	if err != nil {
		return
	}
	traces := r.([]*londobell.TraceMessage)
	
	count32 := 0
	count64 := 0
	for _, v := range traces {
		if v.Detail == nil {
			continue
		}
		switch v.Detail.Method {
		case "PreCommitSector", "ProveCommitSector":
			switch minerType[v.To.Address()] {
			case sector32:
				limit32 = limit32.Add(decimal.NewFromInt(v.GasLimit))
				count32++
			case sector64:
				limit64 = limit64.Add(decimal.NewFromInt(v.GasLimit))
				count64++
			}
		}
	}
	
	if count32 > 0 {
		limit32 = limit32.Div(decimal.NewFromInt(int64(count32)))
	}
	
	if count64 > 0 {
		limit64 = limit64.Div(decimal.NewFromInt(int64(count64)))
	}
	
	return
}
