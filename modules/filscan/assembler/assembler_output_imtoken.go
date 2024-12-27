package assembler

import (
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type IMToken struct {
}

func (t IMToken) BlockToIMToken(source *filscan.BlockDetails) (target *filscan.BlockIMToken) {
	target = &filscan.BlockIMToken{
		Cid:             source.BlockBasic.Cid,
		Epoch:           int(source.BlockBasic.Height.Int64()),
		ParentsWeight:   source.ParentWeight,
		Miner:           source.BlockBasic.MinerID,
		ParentStateRoot: source.StateRoot,
		BlockTime:       int(source.BlockBasic.BlockTime),
		BaseFee:         source.ParentBaseFee,
	}
	return
}

func (t IMToken) MessageToIMToken(message *londobell.MessageTrace, baseFee decimal.Decimal) (target *filscan.MessageIMToken) {
	var gasUsed decimal.Decimal
	if message.GasCost != nil {
		gasUsed = message.GasCost.GasUsed
	}
	if message.Method == "" {
		message.Method = "InvokeEVM"
	}
	target = &filscan.MessageIMToken{
		Cid:        message.Cid,
		From:       message.From.Address(),
		To:         message.To.Address(),
		Nonce:      int(message.Nonce),
		Value:      message.Value,
		GasFeeCap:  message.GasFeeCap,
		GasPremium: message.GasPremium,
		GasLimit:   message.GasLimit,
		Method:     message.Method,
		Exit:       message.ExitCode,
		GasUsed:    gasUsed,
		Epoch:      message.Epoch.Int64(),
		BaseFee:    baseFee,
	}

	return
}

func (t IMToken) BlockListToIMToken(source []*filscan.BlockIMToken) (target map[int][]*filscan.BlockIMToken) {
	newBlockIMToken := make(map[int][]*filscan.BlockIMToken)
	for _, block := range source {
		newBlockIMToken[block.Epoch] = append(newBlockIMToken[block.Epoch], block)
	}
	target = newBlockIMToken
	return
}

func (t IMToken) MessageListToIMToken(source []*filscan.MessageIMToken) (target []*filscan.MessageIMToken) {
	for _, message := range source {
		target = append(target, &filscan.MessageIMToken{
			Cid:        message.Cid,
			From:       message.From,
			To:         message.To,
			Nonce:      message.Nonce,
			Value:      message.Value,
			GasFeeCap:  message.GasFeeCap,
			GasPremium: message.GasPremium,
			GasLimit:   message.GasLimit,
			Method:     message.Method,
			Exit:       message.Exit,
			GasUsed:    message.GasUsed,
			Epoch:      message.Epoch,
			BaseFee:    message.BaseFee,
		})
	}
	target = removeRepForMessageIMToken(target)
	return
}

func removeRepForMessageIMToken(slc []*filscan.MessageIMToken) []*filscan.MessageIMToken {
	var result []*filscan.MessageIMToken // 存放结果
	for i := range slc {
		flag := true
		for j := range result {
			if slc[i].Cid == result[j].Cid {
				flag = false // 存在重复元素，标识为false
				break
			}
		}
		if flag { // 标识为false，不添加进结果
			result = append(result, slc[i])
		}
	}
	return result
}
