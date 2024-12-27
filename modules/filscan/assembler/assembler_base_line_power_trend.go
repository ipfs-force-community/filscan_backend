package assembler

import (
	"sort"

	"github.com/shopspring/decimal"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type BaseLinePowerTrendAssembler struct {
}

func (b BaseLinePowerTrendAssembler) ToBaseLineTrendResponse(epoch chain.Epoch, entities []*bo.BaseLinePower,
	powerAbs []*po.AbsPowerChange) (target *filscan.BaseLineTrendResponse, err error) {
	target = &filscan.BaseLineTrendResponse{
		Epoch:     epoch.Int64(),
		BlockTime: epoch.Time().Unix(),
	}

	sort.Slice(powerAbs, func(i, j int) bool {
		return powerAbs[i].Epoch > powerAbs[j].Epoch
	})
	j := 0
	for i := 0; i < len(entities)-1; i++ {
		c := entities[i]
		p := entities[i+1]
		powerDecrease, powerIncrease := decimal.Zero, decimal.Zero
		for ; j < len(powerAbs); j++ {
			if powerAbs[j].Epoch <= p.Epoch {
				break
			}
			powerDecrease = powerDecrease.Add(powerAbs[j].PowerLoss)
			powerIncrease = powerIncrease.Add(powerAbs[j].PowerIncrease)
		}
		target.List = append(target.List,
			b.ToBaseLineTrend(chain.Epoch(c.Epoch), c,
				c.QualityAdjPower.Sub(p.QualityAdjPower), powerDecrease, powerIncrease))
	}

	return
}

func (BaseLinePowerTrendAssembler) ToBaseLineTrend(epoch chain.Epoch, entity *bo.BaseLinePower,
	increaseQualityAdjPower, powerDecrease, powerIncrease decimal.Decimal) (target *filscan.BaseLineTrend) {
	target = &filscan.BaseLineTrend{
		TotalQualityAdjPower:  entity.QualityAdjPower,
		TotalRawBytePower:     entity.RawBytePower,
		BaseLinePower:         entity.Baseline,
		ChangeQualityAdjPower: increaseQualityAdjPower,
		Timestamp:             epoch.Time().Unix(),
		Epoch:                 epoch,
		PowerDecrease:         powerDecrease,
		PowerIncrease:         powerIncrease,
	}

	return
}
