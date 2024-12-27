package acl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sync"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	message_decode "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/message"
	message_detail "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"

	"github.com/minio/blake2b-simd"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_tool"
)

func NewBlockChainAclImpl(agg AggBlockChainAcl, adapter AdapterBlockChainAcl, fnsQuery repository.NFTQueryer, conf *config.Config) *BlockChainAclImpl {
	return &BlockChainAclImpl{
		agg:      agg,
		adapter:  adapter,
		fnsQuery: fnsQuery,
		redis:    redis.NewRedis(conf),
	}
}

type AggBlockChainAcl interface {
	FinalHeight(ctx context.Context) (epoch *chain.Epoch, err error)
	LatestTipset(ctx context.Context) ([]*londobell.Tipset, error)
	BlockMessages(ctx context.Context, filters types.Filters) (*londobell.BlockMessagesList, error)
	BlockMessagesByMethodName(ctx context.Context, filters types.Filters) (*londobell.MessagesByMethodNameList, error)
	BlocksForMessage(ctx context.Context, cid string) ([]*londobell.BlockHeader, error)
	BatchTraceForMessage(ctx context.Context, start chain.Epoch, cids []string) ([]*londobell.MessageTrace, error)
	MessagesForBlock(ctx context.Context, cid string, filters types.Filters) ([]*londobell.MessageTrace, error)
	MinerBlockReward(ctx context.Context, addr chain.SmartAddress, filter types.Filters) ([]*londobell.MinerBlockReward, error)
	MinersBlockReward(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinersBlockReward, error)
	WinCount(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinerWinCount, error)
	Tipset(ctx context.Context, epoch chain.Epoch) ([]*londobell.Tipset, error)
	Tipsets(ctx context.Context, filters types.Filters) ([]*londobell.Tipset, error)
	TipsetsList(ctx context.Context, filters types.Filters) (result *londobell.TipsetsList, err error)
	TimeOfTrace(ctx context.Context, addr chain.SmartAddress, sort int) ([]*londobell.TimeOfTrace, error)
	TransferLargeAmount(ctx context.Context, filters types.Filters) (*londobell.TransferLargeAmountList, error)
	DealsList(ctx context.Context, filters types.Filters) (*londobell.DealsList, error)
	DealDetails(ctx context.Context, dealId int64) ([]*londobell.DealDetail, error)
	BlockHeader(ctx context.Context, filters types.Filters) (result []*londobell.BlockHeader, err error)
	IncomingBlockHeader(ctx context.Context, filters types.Filters) (result []*londobell.BlockHeader, err error)
	TraceForMessage(ctx context.Context, cid string) ([]*londobell.MessageTrace, error)
	ChildTransfersForMessage(ctx context.Context, cid string) (result []*londobell.MessageTrace, err error)
	ParentTipset(ctx context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error)
	BlockHeaderByCid(ctx context.Context, cid string) ([]*londobell.BlockHeader, error)
	IncomingBlockHeaderByCid(ctx context.Context, cid string) ([]*londobell.BlockHeader, error)
	DealsByAddr(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (*londobell.DealsByAddr, error)
	DealByID(ctx context.Context, dealID int64) ([]*londobell.Deals, error)
	AllMethods(ctx context.Context) ([]*londobell.MethodName, error)
	AllMethodsForBlockMessage(ctx context.Context, cid string) ([]*londobell.MethodName, error)
	MessagesForBlockByMethodName(ctx context.Context, cid string, filters types.Filters) (*londobell.MessagesOfBlock, error)
	MessageCidByHash(ctx context.Context, hash string) (*londobell.CidOrHash, error)
	HashByMessageCid(ctx context.Context, messageCid string) (*londobell.CidOrHash, error)
	MessagePool(ctx context.Context, cid string, filters *types.Filters) (*londobell.MessagePool, error)
	AllMethodsForMessagePool(ctx context.Context) ([]*londobell.MethodName, error)
	BlockMessageForEpochRange(ctx context.Context, start, end chain.Epoch) ([]*londobell.BlockMessageCids, error)
}

type AdapterBlockChainAcl interface {
	Actor(ctx context.Context, actorId chain.SmartAddress, epoch *chain.Epoch) (*londobell.ActorState, error)
	ActorIDs(ctx context.Context, epoch *chain.Epoch) (*londobell.ActorIDs, error)
	Epoch(ctx context.Context, epoch *chain.Epoch) (*londobell.EpochReply, error)
}

type BlockChainAclImpl struct {
	agg      AggBlockChainAcl
	adapter  AdapterBlockChainAcl
	fnsQuery repository.NFTQueryer
	redis    *redis.Redis
}

func (b BlockChainAclImpl) GetFinalHeight(ctx context.Context) (epoch chain.Epoch, err error) {
	finalHeight, err := b.agg.FinalHeight(ctx)
	if err != nil {
		return
	}
	if finalHeight == nil {
		err = fmt.Errorf("final height is empty")
		return
	} else {
		epoch = *finalHeight
	}
	return
}

func (b BlockChainAclImpl) GetLatestTipset(ctx context.Context) (tipset []*londobell.Tipset, err error) {
	latestTipset, err := b.agg.LatestTipset(ctx)
	if err != nil {
		return
	}
	if latestTipset == nil {
		err = fmt.Errorf("latest tipset is empty")
		return
	} else {
		tipset = latestTipset
	}
	return
}

func (b BlockChainAclImpl) GetTipsetDetail(ctx context.Context, height int64) (tipsetDetail *filscan.Tipset, err error) {
	startEpoch := chain.Epoch(height)
	endEpoch := startEpoch + 1
	filters := types.Filters{
		Start: &startEpoch,
		End:   &endEpoch,
	}

	var blocks []*londobell.BlockHeader
	blocks, err = b.agg.BlockHeader(ctx, filters)
	if err != nil {
		return
	}
	var blocksReward []*londobell.MinersBlockReward
	blocksReward, err = b.agg.MinersBlockReward(ctx, *filters.Start, *filters.End)
	if err != nil {
		return
	}
	if blocks == nil || blocksReward == nil {
		return
	}
	mapBlockBasic := make(map[int64][]*filscan.BlockBasic)
	mapMessageCid := make(map[string]struct{})
	bs, err := b.agg.BlockMessageForEpochRange(ctx, chain.Epoch(height), chain.Epoch(height+1))
	if err != nil {
		return nil, err
	}
	mapBlockCids := map[string]*londobell.BlockMessageCids{}
	for i := range bs {
		mapBlockCids[bs[i].BlockCid] = bs[i]
	}
	for _, block := range blocks {
		if _, ok := mapBlockCids[block.ID]; !ok {
			return nil, fmt.Errorf("block cid not matched")
		}
		for i := range mapBlockCids[block.ID].Messages {
			mapMessageCid[mapBlockCids[block.ID].Messages[i]] = struct{}{}
		}
		for _, reward := range blocksReward {
			if reward.Id.Epoch == block.Epoch && reward.Id.Miner == block.Miner {
				newBlockBasic := assembler.BlockChainInfo{}.MinersBlockRewardToBlockBasic(block, reward)
				mapBlockBasic[block.Epoch] = append(mapBlockBasic[block.Epoch], newBlockBasic)
			}
		}
	}
	tipsetDetail = &filscan.Tipset{
		Height:                  big.NewInt(height),
		BlockTime:               uint64(chain.Epoch(height).Unix()),
		MessageCountDeduplicate: int64(len(mapMessageCid)),
		BlockBasic:              mapBlockBasic[height],
	}
	return
}

func (b BlockChainAclImpl) GetTipsetStateTree(ctx context.Context, tfilters types.TipsetFilters) (tipsetState filscan.TipsetStateTreeResponse, err error) {
	var (
		chainBlocks, orphanBlocks []*londobell.BlockHeader
		tipsets                   []*londobell.Tipset
		cerr, oerr, terr          error
		// 包含父块高度的filter
		pfilters    types.Filters
		parentStart int64
		pt          []*londobell.ParentTipset
	)

	// 根据cid,和长度返回tipsetState
	if tfilters.Cid != "" && tfilters.Length > 0 && tfilters.Start == 0 && tfilters.End == 0 {
		var (
			BHRet, IBHRet []*londobell.BlockHeader
		)
		BHRet, err = b.agg.BlockHeaderByCid(ctx, tfilters.Cid)
		if err != nil {
			return
		}
		// 如果不在链上则去查询IncomingBlockHeader
		if len(BHRet) == 0 {
			IBHRet, err = b.agg.IncomingBlockHeaderByCid(ctx, tfilters.Cid)
			if len(IBHRet) == 0 {
				return
			}
			tfilters.End = IBHRet[0].Epoch + 1
		} else {
			tfilters.End = BHRet[0].Epoch + 1
		}

		if tfilters.End-tfilters.Length > 0 {
			tfilters.Start = tfilters.End - tfilters.Length
		} else {
			tfilters.Start = 0
		}

	} else if tfilters.Cid == "" && tfilters.Length == 0 && tfilters.Start != 0 && tfilters.End != 0 {
		// 返回[start,end) tipset tree
		if tfilters.End <= tfilters.Start {
			return tipsetState, fmt.Errorf("err request filter")
		}

	} else {
		return tipsetState, fmt.Errorf("err request filter")
	}

	pt, err = b.agg.ParentTipset(ctx, chain.Epoch(tfilters.Start))
	if err != nil {
		return tipsetState, err
	}

	if len(pt) == 0 {
		parentStart = tfilters.Start
	} else {
		parentStart = pt[0].ID
	}
	pfilters.Start = (*chain.Epoch)(&parentStart)
	pfilters.End = (*chain.Epoch)(&tfilters.End)

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		chainBlocks, cerr = b.agg.BlockHeader(ctx, pfilters)

	}()
	go func() {
		defer wg.Done()
		orphanBlocks, oerr = b.agg.IncomingBlockHeader(ctx, pfilters)

	}()
	go func() {
		defer wg.Done()
		tipsets, terr = b.agg.Tipsets(ctx, pfilters)

	}()
	wg.Wait()
	if cerr != nil {
		return tipsetState, cerr
	}

	if oerr != nil {
		return tipsetState, oerr
	}

	if terr != nil {
		return tipsetState, terr
	}

	tipsetState.TipsetList, err = assembler.GenTipsetStateTree(chainBlocks, orphanBlocks, tipsets, tfilters, parentStart)
	return
}

func (b BlockChainAclImpl) GetLatestBlockList(ctx context.Context, filters types.Filters) (list filscan.LatestBlocksResponse, err error) {
	var totalCount int64
	if filters.Start != nil && filters.InputType != nil && filters.InputType.Value() == types.HEIGHT {
		startEpoch := chain.Epoch(filters.Start.Int64())
		endEpoch := startEpoch + 1
		filters.Start = &startEpoch
		filters.End = &endEpoch
		totalCount = 1
	} else {
		var tipsetsList *londobell.TipsetsList
		tipsetsList, err = b.agg.TipsetsList(ctx, filters)
		if err != nil {
			return
		}
		if tipsetsList == nil {
			return
		}
		totalCount = tipsetsList.TotalCount
		startEpoch := chain.Epoch(tipsetsList.TipSets[len(tipsetsList.TipSets)-1].ID)
		endEpoch := chain.Epoch(tipsetsList.TipSets[0].ID)
		filters.Start = &startEpoch
		filters.End = &endEpoch
	}

	var blocks []*londobell.BlockHeader
	blocks, err = b.agg.BlockHeader(ctx, filters)
	if err != nil {
		return
	}
	var blocksReward []*londobell.MinersBlockReward
	blocksReward, err = b.agg.MinersBlockReward(ctx, *filters.Start, *filters.End)
	if err != nil {
		return
	}
	if blocks == nil || blocksReward == nil {
		return
	}
	mapBlockHeader := make(map[int64][]*londobell.BlockHeader)
	mapBlockBasic := make(map[int64][]*filscan.BlockBasic)
	var mapEpochs []int64
	var convert assembler.BlockChainInfo
	for _, block := range blocks {
		mapEpochs = append(mapEpochs, block.Epoch)
		mapBlockHeader[block.Epoch] = append(mapBlockHeader[block.Epoch], block)
		for _, reward := range blocksReward {
			if reward.Id.Epoch == block.Epoch && reward.Id.Miner == block.Miner {
				newBlockBasic := convert.MinersBlockRewardToBlockBasic(block, reward)
				mapBlockBasic[block.Epoch] = append(mapBlockBasic[block.Epoch], newBlockBasic)
			}
		}
	}

	epochs := _tool.RemoveRepByLoop(mapEpochs)
	if epochs == nil {
		return
	}
	var tipsetList []*filscan.Tipset
	for _, epoch := range epochs {
		var minBlockCid string
		minBlockHeader := b.getMinTicketBlock(mapBlockHeader[epoch])
		if minBlockHeader != nil {
			for i, blockBasic := range mapBlockBasic[epoch] {
				if blockBasic.Cid == minBlockHeader.ID {
					mapBlockBasic[epoch] = _tool.MoveToTopForBlockBasic(mapBlockBasic[epoch], i, 0)
				}
			}
			minBlockCid = minBlockHeader.ID
		} else {
			minBlockCid = ""
		}
		tipsetList = append(tipsetList, &filscan.Tipset{
			Height:         big.NewInt(epoch),
			BlockBasic:     mapBlockBasic[epoch],
			MinTicketBlock: minBlockCid,
		})
	}
	list.BlockBasicList = tipsetList
	list.TotalCount = totalCount

	return
}

func (b BlockChainAclImpl) getMinTicketBlock(blockList []*londobell.BlockHeader) *londobell.BlockHeader {
	minBlock := blockList[0]

	for _, block := range blockList[1:] {
		tDigest := blake2b.Sum256([]byte(block.Ticket.VRFProof.Data))
		oDigest := blake2b.Sum256([]byte(minBlock.Ticket.VRFProof.Data))
		if bytes.Compare(tDigest[:], oDigest[:]) < 0 {
			minBlock = block
		}
	}
	return minBlock
}

func (b BlockChainAclImpl) GetBlockDetails(ctx context.Context, cid string) (blockDetails *filscan.BlockDetails, err error) {
	blockList, err := b.agg.BlockHeaderByCid(ctx, cid)
	if err != nil {
		return
	}
	var block *londobell.BlockHeader
	if blockList != nil {
		block = blockList[0]
		startEpoch := chain.Epoch(block.Epoch)
		endEpoch := startEpoch.Next()
		filters := types.Filters{
			Start: &startEpoch,
			End:   &endEpoch,
			Index: 0,
			Limit: 500,
		}
		var blockReward []*londobell.MinerBlockReward
		blockReward, err = b.agg.MinerBlockReward(ctx, chain.SmartAddress(block.Miner), filters)
		if err != nil {
			return
		}
		var blockWinCount []*londobell.MinerWinCount
		blockWinCount, err = b.agg.WinCount(ctx, chain.Epoch(block.Epoch), chain.Epoch(block.Epoch+1))
		if err != nil {
			return
		}
		var parentTipset []*londobell.ParentTipset
		parentTipset, err = b.agg.ParentTipset(ctx, chain.Epoch(block.Epoch))
		if err != nil {
			return
		}
		var reward *londobell.MinerBlockReward
		var parent *londobell.ParentTipset
		var winCount *londobell.MinerWinCount
		if parentTipset == nil {
			parent = &londobell.ParentTipset{}
		} else {
			parent = parentTipset[0]
		}
		if blockReward != nil && blockWinCount != nil {
			reward = blockReward[0]
			winCount = blockWinCount[0]
		} else {
			return nil, fmt.Errorf("no reward or wincount found")
		}
		var convert assembler.BlockChainInfo
		blockDetails = convert.ToBlockDetails(cid, block, reward, winCount, parent)
	}

	return
}

func (b BlockChainAclImpl) GetMessagesByBlock(ctx context.Context, cid string, filters types.Filters) (messagesByBlock *filscan.MessagesByBlockResponse, err error) {
	blockList, err := b.agg.BlockHeaderByCid(ctx, cid)
	if err != nil {
		return
	}
	var block *londobell.BlockHeader
	if blockList != nil {
		block = blockList[0]
		start := chain.Epoch(block.Epoch)
		filters.Start = &start

		var messageTraceList []*londobell.MessageTrace
		var newMessageByBlock filscan.MessagesByBlockResponse
		if filters.MethodName == "" {
			messageTraceList, err = b.agg.MessagesForBlock(ctx, block.ID, filters)
			if err != nil {
				return
			}
			if messageTraceList != nil {
				newMessageByBlock.TotalCount = block.MessageCount
			} else {
				return
			}
		} else {
			if filters.MethodName == "Other" {
				filters.MethodName = ""
			}
			var messagesCid *londobell.MessagesOfBlock
			messagesCid, err = b.agg.MessagesForBlockByMethodName(ctx, block.ID, filters)
			if err != nil {
				return
			}
			if messagesCid != nil {
				newMessageByBlock.TotalCount = messagesCid.TotalCount
				messageTraceList = messagesCid.Messages
			} else {
				return
			}
		}
		var messageList []*filscan.MessageBasic
		var convert assembler.BlockChainInfo
		for _, message := range messageTraceList {
			newMessage := convert.MessageTraceToMessageBasic(message)
			messageList = append(messageList, newMessage)
		}
		newMessageByBlock.MessageList = messageList
		messagesByBlock = &newMessageByBlock
	}
	return
}

func (b BlockChainAclImpl) GetAllMethodsForBlockMessage(ctx context.Context, cid string) (methodList map[string]int64, err error) {
	var methodNames []*londobell.MethodName
	methodNames, err = b.agg.AllMethodsForBlockMessage(ctx, cid)
	if err != nil {
		return
	}
	methodNameMap := make(map[string]int64)
	if methodNames != nil {
		for _, method := range methodNames {
			if method.ID == "" {
				methodNameMap["Other"] = method.Count
			} else {
				methodNameMap[method.ID] = method.Count
			}
		}
		methodList = methodNameMap
	}

	return
}

func (b BlockChainAclImpl) GetLatestMessageList(ctx context.Context, filters types.Filters) (messageBasicList *filscan.LatestMessagesResponse, err error) {
	var newMessageBasicList filscan.LatestMessagesResponse
	if filters.MethodName == "" {
		var messages *londobell.BlockMessagesList
		messages, err = b.agg.BlockMessages(ctx, filters)
		if err != nil {
			return
		}
		if messages != nil {
			var convert assembler.BlockChainInfo
			for _, message := range messages.BlockMessages {
				messageBasic := convert.BlockMessageToMessageBasic(message)
				newMessageBasicList.MessageList = append(newMessageBasicList.MessageList, messageBasic)
			}
			newMessageBasicList.TotalCount = messages.TotalCount
		}
	} else {
		if filters.MethodName == "Other" {
			filters.MethodName = ""
		}
		var messages *londobell.MessagesByMethodNameList
		messages, err = b.agg.BlockMessagesByMethodName(ctx, filters)
		if err != nil {
			return
		}
		if messages != nil {
			var convert assembler.ActorInfo
			for _, message := range messages.MessagesByMethodName {
				messageBasic := convert.ActorMessageToMessageBasic(message)
				newMessageBasicList.MessageList = append(newMessageBasicList.MessageList, messageBasic)
			}
			newMessageBasicList.TotalCount = messages.TotalCount
		}
	}
	messageBasicList = &newMessageBasicList

	return
}

func (b BlockChainAclImpl) GetMessageDetails(ctx context.Context, cid string) (messageDetails *filscan.MessageDetails, err error) {
	var messageList []*londobell.MessageTrace
	var message *londobell.MessageTrace
	var hash string
	messageList, err = b.agg.ChildTransfersForMessage(ctx, cid)
	if err != nil {
		return
	}
	if messageList == nil {
		messageList, err = b.agg.TraceForMessage(ctx, cid)
		if err != nil {
			return
		}
		if messageList == nil {
			filters := types.Filters{}
			var pendingMessageList *londobell.MessagePool
			pendingMessageList, err = b.agg.MessagePool(ctx, cid, &filters)
			if err != nil {
				return
			}
			var pendingMessage *londobell.PendingMessage
			if pendingMessageList != nil {
				pendingMessage = pendingMessageList.PendingMessage[0]
				hash = pendingMessage.Hash
				var convert assembler.BlockChainInfo
				message = convert.PendingMessageToMessageTrace(pendingMessage)
			} else {
				return
			}
		}
	}
	if messageList != nil {
		message = messageList[0]
	}

	blocksForMessage, err := b.agg.BlocksForMessage(ctx, cid)
	if err != nil {
		return
	}
	var blkCids []string
	var minTicket *londobell.BlockHeader
	if blocksForMessage != nil {
		minTicket = b.getMinTicketBlock(blocksForMessage)
		for _, block := range blocksForMessage {
			blkCids = append(blkCids, block.ID)
		}
	} else {
		blkCids = nil
	}

	var consumeList []*filscan.Consume
	if message.GasCost != nil {
		var convert assembler.BlockChainInfo
		consumeList = convert.GasCostToConsume(message, minTicket)
	}
	var parentEpoch chain.Epoch
	if message.Epoch > 0 {
		parentEpoch = message.Epoch - 1
	}
	tipset, err := b.agg.Tipset(ctx, parentEpoch)
	if err != nil {
		return nil, err
	}
	var baseFee decimal.Decimal
	if tipset != nil {
		baseFee = tipset[0].BaseFee
	}

	if hash == "" {
		var messageHash *londobell.CidOrHash
		messageHash, err = b.agg.HashByMessageCid(ctx, cid)
		if err != nil {
			return
		}
		if messageHash != nil {
			hash = messageHash.Hash
		}
	}

	var epoch chain.Epoch
	if message.ID != 0 {
		epoch = chain.Epoch(message.ID)
	} else {
		epoch = message.Epoch
	}
	var messageParams interface{}
	if message.Params != nil {
		methodName := message_decode.MethodName(message.Method).CheckMethodName(message.To)
		messageParams, err = message_detail.DecodeParamsFromVersion(epoch, message.Params, methodName)
		if err != nil {
			return
		}
	}
	var messageReturns interface{}
	if message.Return != nil {
		messageReturns, err = message_detail.DecodeReturnsFromVersion(epoch, message.Return, message.Method)
		if err != nil {
			return
		}
	}

	var convert assembler.BlockChainInfo
	messageDetail := convert.ToMessageDetails(message, blkCids, consumeList, baseFee, hash, messageParams, messageReturns)
	messageDetails = messageDetail

	return
}

func (b BlockChainAclImpl) GetTransferLargeAmount(ctx context.Context, filters types.Filters) (messageList *filscan.LargeTransfersResponse, err error) {
	transferList, err := b.agg.TransferLargeAmount(ctx, filters)
	if err != nil {
		return
	}

	if transferList != nil {
		var newMessageList filscan.LargeTransfersResponse
		var convert assembler.ActorInfo
		for _, transfer := range transferList.TransferLargeAmount {
			if transfer.Depth != 1 ||
				regexp.MustCompile("^0").MatchString(transfer.From.CrudeAddress()) ||
				regexp.MustCompile("^2").MatchString(transfer.From.CrudeAddress()) {
				transfer.Cid = ""
			}
			message := convert.ActorMessageToMessageBasic(transfer)
			newMessageList.LargeTransferList = append(newMessageList.LargeTransferList, message)
		}
		newMessageList.TotalCount = transferList.TotalCount
		messageList = &newMessageList
	}

	return
}

func (b BlockChainAclImpl) GetDealsList(ctx context.Context, filters types.Filters) (marketDealList *filscan.SearchMarketDealsResponse, err error) {
	var deals *londobell.DealsList
	deals, err = b.agg.DealsList(ctx, filters)
	if err != nil {
		return
	}
	if deals != nil {
		var newMarketDealList filscan.SearchMarketDealsResponse
		var convert assembler.BlockChainInfo
		for _, deal := range deals.DealsList {
			marketDeal := convert.DealsToMarketDeal(deal)
			var adapter *londobell.ActorState
			adapter, err = b.getRedisAdapter(ctx, deal.Client)
			if err != nil {
				return
			}
			if adapter != nil {
				marketDeal.ClientAddress = adapter.ActorAddr
			}
			newMarketDealList.MarketDealsList = append(newMarketDealList.MarketDealsList, marketDeal)
		}
		newMarketDealList.TotalCount = deals.TotalCount
		marketDealList = &newMarketDealList
	}

	return
}

func (b BlockChainAclImpl) GetDealsListByAddr(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (dealDetailsByAddr *filscan.SearchMarketDealsResponse, err error) {
	var dealList *londobell.DealsByAddr
	dealList, err = b.agg.DealsByAddr(ctx, addr, filters)
	if err != nil {
		return
	}
	if dealList != nil {
		var newDealDetailsByAddr filscan.SearchMarketDealsResponse
		var convert assembler.BlockChainInfo
		for _, deal := range dealList.DealsList {
			marketDeal := convert.DealsToMarketDeal(deal)
			var adapter *londobell.ActorState
			adapter, err = b.getRedisAdapter(ctx, deal.Client)
			if err != nil {
				return
			}
			if adapter != nil {
				marketDeal.ClientAddress = adapter.ActorAddr
			}
			newDealDetailsByAddr.MarketDealsList = append(newDealDetailsByAddr.MarketDealsList, marketDeal)
		}
		newDealDetailsByAddr.TotalCount = dealList.TotalCount
		dealDetailsByAddr = &newDealDetailsByAddr
	}

	return
}

func (b BlockChainAclImpl) GetDealByID(ctx context.Context, dealID int64) (marketDeal *filscan.MarketDeal, err error) {
	deal, err := b.agg.DealByID(ctx, dealID)
	if err != nil {
		return
	}
	if deal != nil {
		var convert assembler.BlockChainInfo
		marketDeal = convert.DealsToMarketDeal(deal[0])
		var adapter *londobell.ActorState
		adapter, err = b.getRedisAdapter(ctx, deal[0].Client)
		if err != nil {
			return
		}
		if adapter != nil {
			marketDeal.ClientAddress = adapter.ActorAddr
		}
	}

	return
}

func (b BlockChainAclImpl) GetDealDetails(ctx context.Context, dealID int64) (dealDetails *filscan.DealDetails, err error) {
	dealDetail, err := b.agg.DealDetails(ctx, dealID)
	if err != nil {
		return
	}
	if dealDetail != nil {
		var convert assembler.BlockChainInfo
		dealDetails = convert.DealDetailToDealDetails(dealDetail[0])
	}

	return
}

func (b BlockChainAclImpl) GetTimeOfTrace(ctx context.Context, addr chain.SmartAddress) (LatestTrace *chain.Epoch, err error) {
	traceList, err := b.agg.TimeOfTrace(ctx, addr, -1)
	if err != nil {
		return
	}
	if traceList != nil {
		traceEpoch := chain.Epoch(traceList[0].Epoch)
		LatestTrace = &traceEpoch
	}
	return
}

func (b BlockChainAclImpl) GetAllMethodName(ctx context.Context) (allMethod map[string]int64, err error) {
	var methodNames []*londobell.MethodName
	methodNames, err = b.agg.AllMethods(ctx)
	if err != nil {
		return
	}
	if methodNames != nil {
		methodNameMap := make(map[string]int64)
		for _, method := range methodNames {
			if method.MethodName == "" {
				methodNameMap["Other"] = method.Count
			} else {
				methodNameMap[method.MethodName] = method.Count
			}
		}
		allMethod = methodNameMap
	}

	return
}

func (b BlockChainAclImpl) GetMessagePool(ctx context.Context, cid string, filters *types.Filters) (messagesPool *filscan.MessagesPoolResponse, err error) {
	if filters != nil && filters.MethodName == "Other" {
		filters.MethodName = ""
	}
	messages, err := b.agg.MessagePool(ctx, cid, filters)
	if err != nil {
		return
	}
	if messages != nil {
		var newMessagePool filscan.MessagesPoolResponse
		var convert assembler.BlockChainInfo
		for _, message := range messages.PendingMessage {
			messagePool := convert.PendingMessageToMessagePool(message)
			newMessagePool.MessagesPoolList = append(newMessagePool.MessagesPoolList, messagePool)
		}
		newMessagePool.TotalCount = messages.TotalCount
		messagesPool = &newMessagePool
	}

	return
}

func (b BlockChainAclImpl) GetAllMethodsForMessagePool(ctx context.Context) (allMethod map[string]int64, err error) {
	var methodNames []*londobell.MethodName
	methodNames, err = b.agg.AllMethodsForMessagePool(ctx)
	if err != nil {
		return
	}
	if methodNames != nil {
		methodNameMap := make(map[string]int64)
		for _, method := range methodNames {
			if method.MethodName == "" {
				methodNameMap["Other"] = method.Count
			} else {
				methodNameMap[method.MethodName] = method.Count
			}
		}
		allMethod = methodNameMap
	}

	return
}

func (b BlockChainAclImpl) GetMessageCidByHash(ctx context.Context, hash string) (messageCid string, err error) {
	var messageHash *londobell.CidOrHash
	messageHash, err = b.agg.MessageCidByHash(ctx, hash)
	if err != nil {
		return
	}
	if messageHash != nil {
		messageCid = messageHash.Cid
	}

	return
}

func (b BlockChainAclImpl) GetHashByMessageCid(ctx context.Context, cid string) (hash string, err error) {
	var messageHash *londobell.CidOrHash
	messageHash, err = b.agg.HashByMessageCid(ctx, cid)
	if err != nil {
		return
	}
	if messageHash != nil {
		hash = messageHash.Hash
	}

	return
}

func (b BlockChainAclImpl) SearchFnsTokens(ctx context.Context, name string) (items []*po.FNSToken, err error) {
	items, err = b.fnsQuery.SearchFnsTokens(ctx, name)
	return
}

func (b BlockChainAclImpl) getRedisAdapter(ctx context.Context, actorID chain.SmartAddress) (result *londobell.ActorState, err error) {
	cacheKey, err := b.redis.HexCacheKey(ctx, actorID)
	if err != nil {
		return
	}
	cacheResult, err := b.redis.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &result)
		if err != nil {
			return
		}
		return result, nil
	}
	result, err = b.adapter.Actor(ctx, actorID, nil)
	if err != nil {
		return
	}
	err = b.redis.Set(cacheKey, result, time.Duration(math.MaxInt64))
	if err != nil {
		return
	}
	return
}
