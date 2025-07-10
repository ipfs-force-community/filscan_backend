package trace_task

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type MessageGroup struct {
	messages map[string][]*londobell.TraceMessage
}

func (t *MessageGroup) AddMessageGas(method string, msg *londobell.TraceMessage) {
	if t.messages == nil {
		t.messages = map[string][]*londobell.TraceMessage{}
	}
	t.messages[method] = append(t.messages[method], msg)
}

type MethodGasFee struct {
}

func (m MethodGasFee) parseMessageGas(ctx *TraceContext) (err error) {
	
	group := &MessageGroup{}
	
	for _, v := range ctx.traces {
		if v.GasCost == nil {
			continue
		}
		group.AddMessageGas(v.Detail.Method, v)
	}
	
	for k, v := range group.messages {
		
		count := decimal.NewFromInt(int64(len(v)))
		item := &po.MethodGasFee{
			Epoch:  ctx.Epoch().Int64(),
			Method: k,
			Count:  count.IntPart(),
		}
		
		sumGasPremium := decimal.Decimal{}
		sumGasLimit := decimal.Decimal{}
		sumGasCost := decimal.Decimal{}
		sumGasFee := decimal.Decimal{}
		for _, vv := range v {
			if vv.GasPremium != "" {
				gasPremium, _ := decimal.NewFromString(vv.GasPremium)
				sumGasPremium = sumGasPremium.Add(gasPremium)
			}
			sumGasLimit = sumGasLimit.Add(decimal.NewFromInt(vv.GasLimit))
			sumGasCost = sumGasCost.Add(vv.GasCost.TotalCost)
			sumGasFee = sumGasFee.Add(vv.GasCost.GasUsed)
		}
		
		item.GasPremium = sumGasPremium
		item.GasLimit = sumGasLimit
		item.GasCost = sumGasCost
		item.GasFee = sumGasFee
		
		ctx.methodGas = append(ctx.methodGas, item)
	}
	
	return
}
