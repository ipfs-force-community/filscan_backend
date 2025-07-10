package assembler

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type ActiveMinerTrendAssembler struct {
}

func (ActiveMinerTrendAssembler) ToActiveMinerTrendResponse(epoch chain.Epoch, items []*bo.ActiveMinerCount) (target *filscan.ActiveMinerTrendResponse, err error) {

	target = &filscan.ActiveMinerTrendResponse{
		Epoch:     epoch.Int64(),
		BlockTime: epoch.Time().Unix(),
		Items:     nil,
	}

	for _, v := range items {
		target.Items = append(target.Items, &filscan.ActiveMinerTrend{
			BlockTime:        chain.Epoch(v.Epoch).Time().Unix(),
			ActiveMinerCount: v.ActiveMiners,
		})
	}

	return
}
