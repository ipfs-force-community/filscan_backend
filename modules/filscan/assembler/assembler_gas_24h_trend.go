package assembler

import (
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type GasDataTrendTrendAssembler struct {
}

func (GasDataTrendTrendAssembler) ToGasDataTrend(latest chain.Epoch, entities []*po.MethodGasFee) (target *filscan.GasDataTrendResponse) {
	
	target = &filscan.GasDataTrendResponse{}
	target.Epoch = latest.Int64()
	target.BlockTime = latest.Time().Unix()
	
	var (
		totalCount  decimal.Decimal
		totalGasFee decimal.Decimal
	)
	
	for _, v := range entities {
		totalCount = totalCount.Add(decimal.NewFromInt(v.Count))
		totalGasFee = totalGasFee.Add(v.GasCost)
	}
	
	allCount := decimal.Decimal{}
	allGasPremium := decimal.Decimal{}
	allGasLimit := decimal.Decimal{}
	allGasCost := decimal.Decimal{}
	allGasFee := decimal.Decimal{}
	
	for _, v := range entities {
		if v.Method == "" {
			v.Method = "Other"
		}
		
		allCount = allCount.Add(decimal.NewFromInt(v.Count))
		allGasPremium = allGasPremium.Add(v.GasPremium)
		allGasLimit = allGasLimit.Add(v.GasLimit)
		allGasCost = allGasCost.Add(v.GasCost)
		allGasFee = allGasFee.Add(v.GasFee)
		
		var (
			avgGasPremium decimal.Decimal
			avgGasLimit   decimal.Decimal
			avgGasCost    decimal.Decimal
			avgGasFee     decimal.Decimal
			gasFeeRate    decimal.Decimal
			countRate     decimal.Decimal
		)
		
		count := decimal.NewFromInt(v.Count)
		
		if count.GreaterThan(decimal.Zero) {
			avgGasPremium = v.GasPremium.Div(count)
			avgGasLimit = v.GasLimit.Div(count)
			avgGasCost = v.GasCost.Div(count)
			avgGasFee = v.GasFee.Div(count)
		}
		
		if totalGasFee.GreaterThan(decimal.Zero) {
			gasFeeRate = v.GasCost.Div(totalGasFee)
		}
		if totalCount.GreaterThan(decimal.Zero) {
			countRate = count.Div(totalCount)
		}
		
		target.Items = append(target.Items, &filscan.GasDataTrend{
			MethodName:        v.Method,
			AvgGasPremium:     avgGasPremium,
			AvgGasLimit:       avgGasLimit,
			AvgGasUsed:        avgGasFee.Round(0),
			AvgGasFee:         avgGasCost,
			SumGasFee:         v.GasCost,
			GasFeeRatio:       gasFeeRate,
			MessageCount:      v.Count,
			MessageCountRatio: countRate,
		})
	}
	
	all := &filscan.GasDataTrend{
		MethodName:        "All",
		SumGasFee:         allGasCost,
		GasFeeRatio:       decimal.NewFromFloat(1),
		MessageCount:      allCount.IntPart(),
		MessageCountRatio: decimal.NewFromFloat(1),
	}
	
	if allCount.GreaterThan(decimal.Zero) {
		all.AvgGasPremium = allGasPremium.Div(allCount)
		all.AvgGasLimit = allGasLimit.Div(allCount)
		all.AvgGasUsed = allGasFee.Div(allCount).Round(0)
		all.AvgGasFee = allGasCost.Div(allCount)
	}
	
	target.Items = append([]*filscan.GasDataTrend{all}, target.Items...)
	
	return
}
