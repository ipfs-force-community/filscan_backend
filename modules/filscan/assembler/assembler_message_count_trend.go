package assembler

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type MessageCountTrendAssembler struct {
}

func (MessageCountTrendAssembler) ToMessageCountTrendResponse(epoch chain.Epoch, items []*bo.MessageCount) (target *filscan.MessageCountTrendResponse, err error) {
	
	target = &filscan.MessageCountTrendResponse{
		Epoch:     epoch.Int64(),
		BlockTime: epoch.Time().String(),
		Items:     nil,
	}
	
	for _, v := range items {
		target.Items = append(target.Items, &filscan.MessageCountTrend{
			BlockTime:    chain.Epoch(v.Epoch).Time().String(),
			MessageCount: v.AvgBlockMessage,
			//AllMessageCount: v.AccMessage,
		})
	}
	
	return
	
}
