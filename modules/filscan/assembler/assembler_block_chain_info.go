package assembler

import (
	"math/big"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	message_detail "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/message"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type BlockChainInfo struct {
}

func (b BlockChainInfo) MinerBlockRewardToBlockBasic(source *londobell.BlockHeader, reward *londobell.MinerBlockReward) (target *filscan.BlockBasic) {
	target = &filscan.BlockBasic{
		Height:        big.NewInt(source.Epoch),
		Cid:           source.ID,
		BlockTime:     uint64(chain.Epoch(source.Epoch).Unix()),
		MinerID:       chain.SmartAddress(source.Miner).Address(),
		MessagesCount: source.MessageCount,
		Reward:        reward.TotalBlockReward,
	}
	return
}

func (b BlockChainInfo) MinersBlockRewardToBlockBasic(source *londobell.BlockHeader, reward *londobell.MinersBlockReward) (target *filscan.BlockBasic) {
	target = &filscan.BlockBasic{
		Height:        big.NewInt(source.Epoch),
		Cid:           source.ID,
		BlockTime:     uint64(chain.Epoch(source.Epoch).Unix()),
		MinerID:       chain.SmartAddress(source.Miner).Address(),
		MessagesCount: source.MessageCount,
		Reward:        reward.TotalBlockReward,
	}
	return
}

func (b BlockChainInfo) ToBlockDetails(cid string, block *londobell.BlockHeader, reward *londobell.MinerBlockReward, winCount *londobell.MinerWinCount, parent *londobell.ParentTipset) (target *filscan.BlockDetails) {
	minedReward := reward.TotalBlockReward.Sub(winCount.TotalGasReward)
	var ticketValue string
	if block.Ticket.VRFProof.Binary != nil {
		ticketValue = block.Ticket.VRFProof.Binary.Base64
	} else {
		ticketValue = block.Ticket.VRFProof.Data
	}
	target = &filscan.BlockDetails{
		BlockBasic: filscan.BlockBasic{
			Height:        big.NewInt(block.Epoch),
			Cid:           cid,
			BlockTime:     uint64(chain.Epoch(block.Epoch).Unix()),
			MinerID:       chain.SmartAddress(block.Miner).Address(),
			MessagesCount: block.MessageCount,
			Reward:        reward.TotalBlockReward,
			MinedReward:   &minedReward,
			TxFeeReward:   &winCount.TotalGasReward,
		},
		WinCount:      winCount.TotalWinCount,
		ParentCids:    parent.Cids,
		ParentWeight:  parent.Weight,
		ParentBaseFee: parent.BaseFee,
		TicketValue:   ticketValue,
		StateRoot:     parent.State,
	}
	return
}

// 返回包含(start,end]的数据
func GenTipsetStateTree(chainBlocks, orphanBlocks []*londobell.BlockHeader, tipsets []*londobell.Tipset, filters types.TipsetFilters, parentStart int64) (target []filscan.TipsetState, err error) {

	// 当前高度的tipset信息
	type tipsetData struct {
		Height       int64
		ChainCids    []string
		ChainBlocks  []londobell.BlockHeader
		Weight       string
		OrphanBlocks []londobell.BlockHeader
	}

	tipsetMap := make(map[int64]tipsetData)

	// 通过chainblock构建连续的tipsetMap
	for _, b := range chainBlocks {
		b.Timestamp = chain.Epoch(b.Epoch).Time().Unix()
		b.Miner = chain.SmartAddress(b.Miner).Address()
		if b.FirstSeen == 0 {
			b.FirstSeen = b.Timestamp
		}
		var ts tipsetData
		if _, ok := tipsetMap[b.Epoch]; ok {
			ts = tipsetMap[b.Epoch]
		}
		ts.Height = b.Epoch
		ts.ChainBlocks = append(ts.ChainBlocks, *b)
		ts.ChainCids = append(ts.ChainCids, b.ID)
		tipsetMap[b.Epoch] = ts
	}

	// 更新孤块信息
	for _, b := range orphanBlocks {
		h := b.Epoch
		b.Timestamp = chain.Epoch(b.Epoch).Time().Unix()
		b.Miner = chain.SmartAddress(b.Miner).Address()
		ts := tipsetMap[h]
		var exist bool
		for _, id := range ts.ChainCids {
			if id == b.ID {
				exist = true
			}
		}
		// if !slices.Contains(ts.ChainCids, b.ID) {
		// 	ts.OrphanBlocks = append(ts.OrphanBlocks, *b)
		// }
		if !exist {
			ts.OrphanBlocks = append(ts.OrphanBlocks, *b)
		}
		tipsetMap[h] = ts
	}

	// 更新block weight信息
	for _, lt := range tipsets {
		h := lt.ID
		ts := tipsetMap[h]
		ts.Weight = lt.Weight
		tipsetMap[h] = ts
	}

	var start, end = filters.Start, filters.End

	for cur := end - 1; cur >= start; cur-- {
		var tss filscan.TipsetState
		tss.Height = cur

		tss.OrphanBlocks = tipsetMap[cur].OrphanBlocks
		tss.ChainBlocks = tipsetMap[cur].ChainBlocks

		for i := range tss.ChainBlocks {

			parentHeight := cur - 1
			for parentHeight >= parentStart {
				if len(tipsetMap[parentHeight].ChainBlocks) != 0 {
					tss.ChainBlocks[i].Parents = tipsetMap[parentHeight].ChainCids
					tss.ChainBlocks[i].ParentWeight = tipsetMap[parentHeight].Weight
					break
				}
				parentHeight--
			}
		}

		target = append(target, tss)
	}
	return
}

func (b BlockChainInfo) MessageTraceToMessageBasic(source *londobell.MessageTrace) (target *filscan.MessageBasic) {
	target = &filscan.MessageBasic{
		Height:     big.NewInt(source.Epoch.Int64()),
		BlockTime:  uint64(source.Epoch.Unix()),
		Cid:        source.Cid,
		From:       source.From.Address(),
		To:         source.To.Address(),
		Value:      source.Value,
		ExitCode:   message_detail.ExitCode(source.ExitCode).String(),
		MethodName: source.Method,
	}
	return
}

func (b BlockChainInfo) BlockMessageToMessageBasic(source *londobell.BlockMessage) (target *filscan.MessageBasic) {
	target = &filscan.MessageBasic{
		Height:     big.NewInt(source.Epoch),
		BlockTime:  uint64(chain.Epoch(source.Epoch).Unix()),
		Cid:        source.SignedCid,
		From:       source.From.Address(),
		To:         source.To.Address(),
		Value:      source.Value,
		ExitCode:   message_detail.ExitCode(source.ExitCode).String(),
		MethodName: source.Method,
	}
	if target.MethodName == "" {
		target.MethodName = "Other"
	}
	return
}

func (b BlockChainInfo) PendingMessageToMessageTrace(source *londobell.PendingMessage) (target *londobell.MessageTrace) {
	target = &londobell.MessageTrace{
		Cid:        source.SignedCid.String(),
		Epoch:      source.Epoch,
		Value:      source.Value,
		From:       source.From,
		To:         source.To,
		Method:     source.Method,
		ExitCode:   -1,
		GasLimit:   source.GasLimit,
		GasPremium: source.GasPremium,
		GasCost:    &londobell.GasCost{},
	}
	return
}

func (b BlockChainInfo) GasCostToConsume(source *londobell.MessageTrace, minTicket *londobell.BlockHeader) (target []*filscan.Consume) {
	var consume *filscan.Consume
	if !source.GasCost.MinerTip.IsZero() && minTicket != nil {
		consume = &filscan.Consume{
			From:        source.From.Address(),
			To:          chain.SmartAddress(minTicket.Miner).Address(),
			Value:       source.GasCost.MinerTip,
			ConsumeType: "MinerTip",
		}
		target = append(target, consume)
	}
	if !source.GasCost.BaseFeeBurn.IsZero() {
		consume = &filscan.Consume{
			From:        source.From.Address(),
			To:          chain.SmartAddress("099").Address(),
			Value:       source.GasCost.BaseFeeBurn.Add(source.GasCost.OverEstimationBurn),
			ConsumeType: "BaseFeeBurn",
		}
		target = append(target, consume)
	}
	if !source.Value.IsZero() {
		consume = &filscan.Consume{
			From:        source.From.Address(),
			To:          source.To.Address(),
			Value:       source.Value,
			ConsumeType: "Transfer",
		}
		target = append(target, consume)
	}
	if source.TransferList != nil {
		for _, transfer := range source.TransferList {
			if transfer.To.Address() == chain.SmartAddress("099").Address() {
				consume = &filscan.Consume{
					From:        transfer.From.Address(),
					To:          transfer.To.Address(),
					Value:       transfer.Value,
					ConsumeType: "Burn",
				}
				target = append(target, consume)
			}
		}
	}

	return
}

func (b BlockChainInfo) ToMessageDetails(message *londobell.MessageTrace, blkCids []string, consumeList []*filscan.Consume, baseFee decimal.Decimal, hash string, messageParam interface{}, messageReturn interface{}) (target *filscan.MessageDetails) {
	target = &filscan.MessageDetails{
		MessageBasic: filscan.MessageBasic{
			Height:     big.NewInt(message.Epoch.Int64()),
			BlockTime:  uint64(message.Epoch.Unix()),
			Cid:        message.Cid,
			From:       message.From.Address(),
			To:         message.To.Address(),
			Value:      message.Value,
			ExitCode:   message_detail.ExitCode(message.ExitCode).String(),
			MethodName: message.Method,
		},
		BlkCids:       blkCids,
		ConsumeList:   consumeList,
		Version:       message.Version,
		Nonce:         uint64(message.Nonce),
		GasFeeCap:     message.GasFeeCap,
		GasPremium:    message.GasPremium,
		GasLimit:      message.GasLimit,
		GasUsed:       message.GasCost.GasUsed,
		BaseFee:       baseFee,
		AllGasFee:     message.GasCost.TotalCost,
		ParamsDetail:  messageParam,
		ReturnsDetail: messageReturn,
		ETHMessage:    hash,
		Error:         message.Error,
	}
	if message.ID != 0 {
		target.MessageBasic.Height = big.NewInt(message.ID)
		target.MessageBasic.BlockTime = uint64(chain.Epoch(message.ID).Unix())
	}
	return
}

func (b BlockChainInfo) ActorMessagesToMessageBasic(source *londobell.ActorMessages) (target *filscan.MessageBasic) {
	target = &filscan.MessageBasic{
		Height:     big.NewInt(source.Epoch),
		BlockTime:  uint64(chain.Epoch(source.Epoch).Unix()),
		Cid:        source.Cid,
		From:       source.From.Address(),
		To:         source.To.Address(),
		Value:      source.Value,
		MethodName: source.Method,
	}
	return
}

func (b BlockChainInfo) DealsToMarketDeal(source *londobell.Deals) (target *filscan.MarketDeal) {
	target = &filscan.MarketDeal{
		DealID:                source.ID,
		PieceCid:              source.PieceCID,
		PieceSize:             decimal.NewFromInt(source.PieceSize),
		ClientAddress:         source.Client.Address(),
		ProviderID:            source.Provider.Address(),
		StartHeight:           big.NewInt(source.StartEpoch),
		StartTime:             chain.Epoch(source.StartEpoch).Unix(),
		EndHeight:             big.NewInt(source.EndEpoch),
		EndTime:               chain.Epoch(source.EndEpoch).Unix(),
		StoragePricePerHeight: source.StoragePricePerEpoch,
		VerifiedDeal:          source.VerifiedDeal,
	}
	return
}

func (b BlockChainInfo) DealDetailToDealDetails(source *londobell.DealDetail) (target *filscan.DealDetails) {
	target = &filscan.DealDetails{
		DealID:               source.DealID,
		Epoch:                source.Epoch,
		MessageCid:           source.Cid,
		PieceCid:             source.PieceCID,
		VerifiedDeal:         false,
		PieceSize:            source.PieceSize,
		Client:               source.Client.Address(),
		ClientCollateral:     source.ClientCollateral,
		Provider:             source.Provider.Address(),
		ProviderCollateral:   source.ProviderCollateral,
		StartEpoch:           source.StartEpoch,
		StartTime:            chain.Epoch(source.StartEpoch).Unix(),
		EndEpoch:             source.EndEpoch,
		EndTime:              chain.Epoch(source.EndEpoch).Unix(),
		StoragePricePerEpoch: source.StoragePricePerEpoch,
	}
	return
}

func (b BlockChainInfo) PendingMessageToMessagePool(source *londobell.PendingMessage) (target *filscan.MessagePool) {
	target = &filscan.MessagePool{
		MessageBasic: filscan.MessageBasic{
			Height:     big.NewInt(source.Epoch.Int64()),
			BlockTime:  uint64(source.MsgTime),
			Cid:        source.SignedCid.String(),
			From:       source.From.Address(),
			To:         source.To.Address(),
			Value:      source.Value,
			MethodName: source.Method,
		},
		GasLimit:   source.GasLimit,
		GasPremium: source.GasPremium,
	}
	return
}
