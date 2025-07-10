package convertor

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type BaseGasCostConvertor struct {
}

func (BaseGasCostConvertor) ToBaseGasCostPo(source *stat.BaseGasCost) (target *po.BaseGasCostPo, err error) {
	
	target = &po.BaseGasCostPo{
		Epoch:         source.Epoch.Int64(),
		BaseGas:       source.BaseGas.Decimal(),
		SectorGas32:   source.SectorGas32.Decimal(),
		SectorGas64:   source.SectorGas64.Decimal(),
		Messages:      source.Messages,
		AccMessages:   source.AccMessages,
		AvgGasLimit32: source.AvgGasLimit32,
		AvgGasLimit64: source.AvgGasLimit64,
		SectorFee32:   source.SectorFee32,
		SectorFee64:   source.SectorFee64,
	}
	
	return
}

func (BaseGasCostConvertor) ToBaseGasCostEntity(source *po.BaseGasCostPo) (target *stat.BaseGasCost, err error) {
	target = &stat.BaseGasCost{
		Epoch:         chain.Epoch(source.Epoch),
		BaseGas:       chain.AttoFil(source.BaseGas),
		SectorGas32:   chain.AttoFil(source.SectorGas32),
		SectorGas64:   chain.AttoFil(source.SectorGas64),
		Messages:      source.Messages,
		AccMessages:   source.AccMessages,
		AvgGasLimit32: source.AvgGasLimit32,
		AvgGasLimit64: source.AvgGasLimit64,
		SectorFee32:   source.SectorFee32,
		SectorFee64:   source.SectorFee64,
	}
	return
}
