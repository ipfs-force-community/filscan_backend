package acl

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/gozelle/async/parallel"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

func NewAccountAclImpl(agg AggAccountAcl, adapter AdapterAccountAcl) *AccountAclImpl {
	return &AccountAclImpl{
		agg:     agg,
		adapter: adapter,
	}
}

type AggAccountAcl interface {
	FinalHeight(ctx context.Context) (epoch *chain.Epoch, err error)
	LatestTipset(ctx context.Context) ([]*londobell.Tipset, error)
	Address(ctx context.Context, addr chain.SmartAddress) (*londobell.Address, error)
	MinerGasCost(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinerGasCost, error)
	MinerBlockReward(ctx context.Context, addr chain.SmartAddress, filter types.Filters) ([]*londobell.MinerBlockReward, error)
	WinCount(ctx context.Context, begin, end chain.Epoch) ([]*londobell.MinerWinCount, error)
	ActorMessages(ctx context.Context, addr chain.SmartAddress, filter types.Filters) (*londobell.ActorMessagesList, error)
	ActorMessagesByMethodName(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (*londobell.MessagesByMethodNameList, error)
	TransferMessages(ctx context.Context, addr chain.SmartAddress, filter types.Filters) (*londobell.TransferMessagesList, error)
	TransferMessagesByEpoch(ctx context.Context, addr chain.SmartAddress, filter types.Filters) (*londobell.TransferMessagesList, error)
	TimeOfTrace(ctx context.Context, addr chain.SmartAddress, sort int) ([]*londobell.TimeOfTrace, error)
	MinerCreateTime(ctx context.Context, addr chain.SmartAddress, to string, method int) (*londobell.TimeOfTrace, error)
	BlockHeadersByMiner(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (*londobell.BlockHeadersByMiner, error)
	AllMethodsForActor(ctx context.Context, addr chain.SmartAddress) ([]*londobell.MethodName, error)
	ActorBalance(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) (result []*londobell.ActorBalance, err error)
}

type AdapterAccountAcl interface {
	Epoch(ctx context.Context, epoch *chain.Epoch) (*londobell.EpochReply, error)
	Actor(ctx context.Context, actorId chain.SmartAddress, epoch *chain.Epoch) (*londobell.ActorState, error)
	Miner(ctx context.Context, miner chain.SmartAddress, epoch *chain.Epoch) (*londobell.MinerDetail, error)
}

type AccountAclImpl struct {
	agg     AggAccountAcl
	adapter AdapterAccountAcl
}

func (a AccountAclImpl) CheckActorAddress(ctx context.Context, id chain.SmartAddress) (addr *londobell.Address, err error) {
	address, err := a.agg.Address(ctx, id)
	if err != nil {
		return
	}
	if address != nil {
		addr = address
	}
	return
}

func (a AccountAclImpl) GetHeight(ctx context.Context) (epoch chain.Epoch, err error) {
	r, err := a.adapter.Epoch(ctx, nil)
	if err != nil {
		return
	}
	if r == nil {
		err = fmt.Errorf("query node epoch error: %s", err)
		return
	}
	epoch = chain.Epoch(r.Epoch)
	return
}

func (a AccountAclImpl) GetAccountBasic(ctx context.Context, actorId chain.SmartAddress, epoch chain.Epoch) (accountBasic *filscan.AccountBasic, err error) {

	actor, err := a.adapter.Actor(ctx, actorId, &epoch)
	if err != nil {
		if strings.Contains(err.Error(), "actor not found") {
			return nil, nil
		}
		return
	}
	if actor != nil {
		var convert assembler.ActorInfo
		accountBasic = convert.ActorStateToAccountBasic(actor)
	}
	return
}

func (a AccountAclImpl) GetMultiSignBasic(ctx context.Context, actorId chain.SmartAddress) (signersBasic *filscan.AccountMultisig, err error) {
	actor, err := a.adapter.Actor(ctx, actorId, nil)
	if err != nil {
		return
	}
	if actor != nil {
		var convert assembler.ActorInfo
		signersBasic, err = convert.ActorStateToAccountSigners(actor)
		if err != nil {
			return
		}
		var signerList []string
		if signersBasic.Signers != nil {
			for _, controller := range signersBasic.Signers {
				var signerAddress *londobell.Address
				signerAddress, err = a.agg.Address(ctx, chain.SmartAddress(controller))
				if err != nil {
					return
				}
				if signerAddress != nil {
					signerList = append(signerList, chain.SmartAddress(signerAddress.RobustAddress).Address())
				}
			}
			signersBasic.Signers = signerList
		}
	}
	return
}

func (a AccountAclImpl) GetAccountMinerInfo(ctx context.Context, actorId chain.SmartAddress) (accountMiner *filscan.AccountMiner, err error) {
	miner, err := a.adapter.Miner(ctx, actorId, nil)
	if err != nil {
		return
	}
	if miner != nil {
		var ownerActor *londobell.ActorState
		ownerActor, err = a.adapter.Actor(ctx, chain.SmartAddress(miner.Owner), nil)
		if err != nil {
			return
		}
		if ownerActor != nil && ownerActor.ActorType != types.MULTISIG {
			var ownerAddress *londobell.Address
			ownerAddress, err = a.agg.Address(ctx, chain.SmartAddress(miner.Owner))
			if err != nil {
				return
			}
			if ownerAddress != nil {
				miner.Owner = chain.SmartAddress(ownerAddress.RobustAddress).Address()
			}
		}

		var workerAddress *londobell.Address
		workerAddress, err = a.agg.Address(ctx, chain.SmartAddress(miner.Worker))
		if err != nil {
			return
		}
		if workerAddress != nil {
			miner.Worker = chain.SmartAddress(workerAddress.RobustAddress).Address()
		}
		var newControllerList []string
		if miner.Controllers != nil {
			for _, controller := range miner.Controllers {
				var controllerAddress *londobell.Address
				controllerAddress, err = a.agg.Address(ctx, chain.SmartAddress(controller))
				if err != nil {
					return
				}
				if controllerAddress != nil {
					newControllerList = append(newControllerList, chain.SmartAddress(controllerAddress.RobustAddress).Address())
				}
			}
			miner.Controllers = newControllerList
		}

		var beneAddress *londobell.Address
		beneAddress, err = a.agg.Address(ctx, chain.SmartAddress(miner.Beneficiary))
		if err != nil {
			return
		}
		if beneAddress != nil {
			miner.Beneficiary = chain.SmartAddress(beneAddress.RobustAddress).Address()
		}
		var convert assembler.ActorInfo
		accountMiner = convert.MinerDetailToAccountMiner(miner)
	}
	return
}

func (a AccountAclImpl) GetOwnedMinersInfo(ctx context.Context, ownedMinersID []chain.SmartAddress) (accountMinerList []*filscan.AccountMiner, err error) {
	var runners []parallel.Runner[*filscan.AccountMiner]
	for _, item := range ownedMinersID {
		ownedMinerID := item
		runners = append(runners, func(_ context.Context) (result *filscan.AccountMiner, err error) {
			result, err = a.GetAccountMinerInfo(ctx, ownedMinerID)
			if err != nil {
				return
			}
			return result, nil
		})
	}
	results := parallel.Run[*filscan.AccountMiner](context.TODO(), 30, runners)
	err = parallel.Wait[*filscan.AccountMiner](results, func(v *filscan.AccountMiner) error {
		accountMinerList = append(accountMinerList, v)
		return nil
	})
	if err != nil {
		return
	}
	return
}

func (a AccountAclImpl) GetMinerWinCounts(ctx context.Context, epochList []int64) (actorStateList []*londobell.ActorState, err error) {
	var runners []parallel.Runner[*londobell.ActorState]
	actorID := chain.SmartAddress("02")
	for _, item := range epochList {
		epoch := chain.Epoch(item)
		runners = append(runners, func(_ context.Context) (result *londobell.ActorState, err error) {
			result, err = a.adapter.Actor(ctx, actorID, &epoch)
			if err != nil {
				return
			}

			return result, nil
		})
	}

	results := parallel.Run[*londobell.ActorState](context.TODO(), 100, runners)
	err = parallel.Wait[*londobell.ActorState](results, func(v *londobell.ActorState) error {
		actorStateList = append(actorStateList, v)
		return nil
	})
	if err != nil {
		return
	}

	return
}

//func (a AccountAclImpl) GetMinerIndicator(ctx context.Context, actorId chain.SmartAddress, input types.IntervalType) (minerIndicators *filscan.MinerIndicators, err error) {
//	tipset, err := a.agg.LatestTipset(ctx)
//	if err != nil {
//		return nil, err
//	}
//	if tipset != nil {
//		var filters types.Filters
//		endEpoch := chain.Epoch(tipset[0].ID)
//		filters.End = &endEpoch
//		var dayCount decimal.Decimal
//		switch input.Value() {
//		case types.DAY:
//			startEpoch := chain.Epoch(tipset[0].ID - 2880)
//			filters.Start = &startEpoch
//			dayCount = decimal.NewFromInt(1)
//		case types.WEEK:
//			startEpoch := chain.Epoch(tipset[0].ID - (2880 * 7))
//			filters.Start = &startEpoch
//			dayCount = decimal.NewFromInt(7)
//		case types.MONTH:
//			startEpoch := chain.Epoch(tipset[0].ID - (2880 * 30))
//			filters.Start = &startEpoch
//			dayCount = decimal.NewFromInt(30)
//		}
//		var minerEndDetail *londobell.MinerDetail
//		minerEndDetail, err = a.adapter.Miner(ctx, actorId, filters.End)
//		if err != nil {
//			return
//		}
//		var minerStartDetail *londobell.MinerDetail
//		minerStartDetail, err = a.adapter.Miner(ctx, actorId, filters.Start)
//		if err != nil {
//			return
//		}
//		if minerEndDetail != nil && minerStartDetail != nil {
//			minerIndicators.PowerIncrease = minerEndDetail.QualityPower.Div(minerStartDetail.QualityPower)
//			minerIndicators.PowerRatio = minerIndicators.PowerIncrease.Div(dayCount)
//			minerIndicators.SectorIncrease = decimal.NewFromInt(minerEndDetail.SectorCount - minerStartDetail.SectorCount).Mul(decimal.NewFromInt(minerEndDetail.SectorSize))
//			minerIndicators.SectorRatio = minerIndicators.SectorIncrease.Div(dayCount)
//			minerIndicators.SectorDeposits = minerEndDetail.InitialPledgeRequirement.Div(minerStartDetail.InitialPledgeRequirement)
//		}
//		var minerBlockHeaders []*londobell.MinerBlockReward
//		minerBlockHeaders, err = a.agg.MinerBlockReward(ctx, actorId, filters)
//		if err != nil {
//			return
//		}
//		if minerBlockHeaders != nil {
//			minerIndicators.BlockRewardIncrease = minerBlockHeaders[0].TotalBlockReward
//			minerIndicators.BlockCountIncrease = minerBlockHeaders[0].BlockCount
//		}
//		var minerWinCounts []*londobell.MinerWinCount
//		minerWinCounts, err = a.agg.WinCount(ctx, actorId, filters)
//		if err != nil {
//			return nil, err
//		}
//		var minerGasCost []*londobell.MinerGasCost
//		minerGasCost, err = a.agg.MinerGasCost(ctx, actorId, filters)
//		if err != nil {
//			return nil, err
//		}
//
//	}
//
//	return
//}

func (a AccountAclImpl) GetAccountBalanceTrend(ctx context.Context, actorId chain.SmartAddress, input types.IntervalType) (balanceTrend []*filscan.BalanceTrend, err error) {
	finalHeight, err := a.agg.FinalHeight(ctx)
	if err != nil {
		return
	}
	if finalHeight != nil {
		currentDay := finalHeight.CurrentDay()
		var resolveInterval interval.Interval
		resolveInterval, err = interval.ResolveInterval(string(input), currentDay)
		if err != nil {
			return
		}
		var epochList []chain.Epoch
		epochList = resolveInterval.Points()

		for _, epoch := range epochList {
			var balance []*londobell.ActorBalance
			balance, err = a.agg.ActorBalance(ctx, epoch, actorId)
			if err != nil {
				return
			}
			var newBalance *filscan.BalanceTrend
			if balance != nil {
				var convert assembler.ActorInfo
				newBalance = convert.ActorBalanceToBalanceTrend(balance[0])
			}
			if newBalance != nil {
				balanceTrend = append(balanceTrend, newBalance)
			}
		}
	}
	return
}

func (a AccountAclImpl) GetActorBlocks(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (actorBlocks *filscan.BlocksByAccountIDResponse, err error) {
	var blocks *londobell.BlockHeadersByMiner
	blocks, err = a.agg.BlockHeadersByMiner(ctx, addr, filters)
	if err != nil {
		return
	}
	if blocks != nil {
		startEpoch := chain.Epoch(blocks.BlockHeaders[len(blocks.BlockHeaders)-1].Epoch)
		endEpoch := chain.Epoch(blocks.BlockHeaders[0].Epoch + 1)
		filters.Start = &startEpoch
		filters.End = &endEpoch
		var blockRewards []*londobell.MinerBlockReward
		blockRewards, err = a.agg.MinerBlockReward(ctx, addr, filters)
		if err != nil {
			return
		}
		if blockRewards != nil {
			var blockList filscan.BlocksByAccountIDResponse
			var convert assembler.BlockChainInfo
			for _, block := range blocks.BlockHeaders {
				for _, reward := range blockRewards {
					if block.Epoch == reward.Id {
						newBlock := convert.MinerBlockRewardToBlockBasic(block, reward)
						blockList.BlocksByAccountIDList = append(blockList.BlocksByAccountIDList, newBlock)
					}
				}
			}
			blockList.TotalCount = blocks.TotalCount
			actorBlocks = &blockList
		}
	}

	return
}

func (a AccountAclImpl) GetActorMessages(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (actorMessagesList *filscan.MessagesByAccountIDResponse, err error) {
	var messageList filscan.MessagesByAccountIDResponse
	var convertor assembler.ActorInfo
	if filters.MethodName == "" {
		var messages *londobell.ActorMessagesList
		messages, err = a.agg.ActorMessages(ctx, addr, filters)
		if err != nil {
			return
		}
		if messages != nil {
			for _, message := range messages.ActorMessages {
				messageBasic := convertor.ActorMessageToMessageBasic(message)
				messageList.MessagesByAccountIDList = append(messageList.MessagesByAccountIDList, messageBasic)
			}
			messageList.TotalCount = messages.TotalCount
		}
	} else {
		var messages *londobell.MessagesByMethodNameList
		messages, err = a.agg.ActorMessagesByMethodName(ctx, addr, filters)
		if err != nil {
			return
		}
		if messages != nil {
			for _, message := range messages.MessagesByMethodName {
				messageBasic := convertor.ActorMessageToMessageBasic(message)
				messageList.MessagesByAccountIDList = append(messageList.MessagesByAccountIDList, messageBasic)
			}
			messageList.TotalCount = messages.TotalCount
		}
	}
	actorMessagesList = &messageList
	return
}

func (a AccountAclImpl) GetActorTransfers(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (actorTransfers *filscan.TracesByAccountIDResponse, err error) {
	var transfers *londobell.TransferMessagesList
	transfers, err = a.agg.TransferMessages(ctx, addr, filters)
	if err != nil {
		return
	}
	actorState, err := a.adapter.Actor(ctx, addr, nil)
	if err != nil {
		return nil, err
	}

	if transfers != nil {
		var traceList filscan.TracesByAccountIDResponse
		var convertor assembler.ActorInfo
		for _, transfer := range transfers.TransferMessages {
			if transfer.Depth != 1 ||
				regexp.MustCompile("^0").MatchString(transfer.From.CrudeAddress()) ||
				regexp.MustCompile("^2").MatchString(transfer.From.CrudeAddress()) {
				transfer.Cid = ""
			}
			if transfer.From.Address() == actorState.ActorID || transfer.From.Address() == actorState.ActorAddr || transfer.From.Address() == actorState.DelegatedAddr {
				transfer.Method = "Send"
				if transfer.To.CrudeAddress() == "099" {
					transfer.Method = "Burn"
				}
			} else if transfer.To.Address() == actorState.ActorID || transfer.To.Address() == actorState.ActorAddr || transfer.To.Address() == actorState.DelegatedAddr {
				transfer.Method = "Receive"
				if transfer.From.CrudeAddress() == "02" {
					transfer.Method = "Blockreward"
				}
			} else {
				actorStateTo, err := a.adapter.Actor(ctx, transfer.To, nil)
				if err != nil {
					return nil, err
				}
				if actorStateTo.ActorID == actorState.ActorID {
					transfer.Method = "Receive"
					if transfer.From.CrudeAddress() == "02" {
						transfer.Method = "Blockreward"
					}
				} else {
					transfer.Method = "Send"
					if transfer.To.CrudeAddress() == "099" {
						transfer.Method = "Burn"
					}
				}
			}
			newTransfer := convertor.ActorMessageToMessageBasic(transfer)
			traceList.TracesByAccountIDList = append(traceList.TracesByAccountIDList, newTransfer)
		}
		traceList.TotalCount = transfers.TotalCount
		actorTransfers = &traceList
	}

	return
}

func (a AccountAclImpl) GetActorTransfersForIMToken(ctx context.Context, addr chain.SmartAddress, filters types.Filters) (actorTransfers *filscan.TracesByAccountIDResponse, err error) {
	var transfers *londobell.TransferMessagesList
	transfers, err = a.agg.TransferMessagesByEpoch(ctx, addr, filters)
	if err != nil {
		return
	}
	actorState, err := a.adapter.Actor(ctx, addr, nil)
	if err != nil {
		return nil, err
	}

	if transfers != nil {
		var traceList filscan.TracesByAccountIDResponse
		var convertor assembler.ActorInfo
		for _, transfer := range transfers.TransferMessages {
			if regexp.MustCompile("^0").MatchString(transfer.From.CrudeAddress()) ||
				regexp.MustCompile("^2").MatchString(transfer.From.CrudeAddress()) {
				transfer.Cid = ""
			}
			if regexp.MustCompile("^0").MatchString(transfer.From.CrudeAddress()) {
				transfer.From = chain.SmartAddress(actorState.ActorAddr)
			} else if regexp.MustCompile("^0").MatchString(transfer.To.CrudeAddress()) {
				transfer.To = chain.SmartAddress(actorState.ActorAddr)
			}
			if transfer.From.Address() == actorState.ActorID || transfer.From.Address() == actorState.ActorAddr || transfer.From.Address() == actorState.DelegatedAddr {
				transfer.Method = "Send"
				if transfer.To.CrudeAddress() == "099" {
					transfer.Method = "Burn"
				}
			} else if transfer.To.Address() == actorState.ActorID || transfer.To.Address() == actorState.ActorAddr || transfer.To.Address() == actorState.DelegatedAddr {
				transfer.Method = "Receive"
				if transfer.From.CrudeAddress() == "02" {
					transfer.Method = "Blockreward"
				}
			} else {
				actorStateTo, err := a.adapter.Actor(ctx, transfer.To, nil)
				if err != nil {
					return nil, err
				}
				if actorStateTo.ActorID == actorState.ActorID {
					transfer.Method = "Receive"
					if transfer.From.CrudeAddress() == "02" {
						transfer.Method = "Blockreward"
					}
				} else {
					transfer.Method = "Send"
					if transfer.To.CrudeAddress() == "099" {
						transfer.Method = "Burn"
					}
				}
			}
			newTransfer := convertor.ActorMessageToMessageBasic(transfer)
			traceList.TracesByAccountIDList = append(traceList.TracesByAccountIDList, newTransfer)
		}
		traceList.TotalCount = transfers.TotalCount
		actorTransfers = &traceList
	}

	return
}

func (a AccountAclImpl) GetAllMethodNameByID(ctx context.Context, addr chain.SmartAddress) (allMethod map[string]int64, err error) {
	var methodNames []*londobell.MethodName
	methodNames, err = a.agg.AllMethodsForActor(ctx, addr)
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
