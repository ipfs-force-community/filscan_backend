package assembler

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type BaseFeeTrendAssembler struct {
}

func (b BaseFeeTrendAssembler) ToBaseFeeTrendResponse(latest chain.Epoch, entities []*stat.BaseGasCost) (target *filscan.BaseFeeTrendResponse, err error) {
	target = &filscan.BaseFeeTrendResponse{}
	target.Epoch = latest.Int64()
	target.BlockTime = latest.Time().Unix()
	for _, v := range entities {
		target.List = append(target.List, b.ToBaseFeeTrend(v))
	}
	
	return
}

func (BaseFeeTrendAssembler) ToBaseFeeTrend(entity *stat.BaseGasCost) (target filscan.BaseFeeTrend) {
	target.BaseFee = entity.BaseGas.Decimal()
	target.Timestamp = entity.Epoch.Time().String()
	//target.GasIn32G = entity.SectorGas32.Decimal()
	//target.GasIn64G = entity.SectorGas64.Decimal()
	target.GasIn32G = entity.SectorFee32
	target.GasIn64G = entity.SectorFee64
	return
}
