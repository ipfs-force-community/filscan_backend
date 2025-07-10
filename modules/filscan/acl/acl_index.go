package acl

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

type AggIndexAcl interface {
	FinalHeight(ctx context.Context) (epoch *chain.Epoch, err error)
	MinersBlockReward(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinersBlockReward, error)
	LatestTipset(ctx context.Context) ([]*londobell.Tipset, error)
	ActorStateEpoch(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) ([]*londobell.ActorStateEpoch, error)
	MinerInfo(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) ([]*londobell.MinerInfo, error)
	MinerGasCost(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinerGasCost, error)
	BlockHeader(ctx context.Context, filters types.Filters) (result []*londobell.BlockHeader, err error)
	CountOfBlockMessages(ctx context.Context, start, end chain.Epoch) (count int64, err error)
}

type AdapterIndexAcl interface {
	Actor(ctx context.Context, actorId chain.SmartAddress, epoch *chain.Epoch) (*londobell.ActorState, error)
	Miner(ctx context.Context, miner chain.SmartAddress, epoch *chain.Epoch) (*londobell.MinerDetail, error)
	CurrentSectorInitialPledge(ctx context.Context, epoch *chain.Epoch) (*londobell.CurrentSectorInitialPledge, error)
	Epoch(ctx context.Context, epoch *chain.Epoch) (*londobell.EpochReply, error)
}

func NewIndexAclImpl(agg AggIndexAcl, adapter AdapterIndexAcl) *IndexAclImpl {
	return &IndexAclImpl{agg: agg, adapter: adapter}
}

type IndexAclImpl struct {
	agg     AggIndexAcl
	adapter AdapterIndexAcl
}

func (a IndexAclImpl) CountOfBlockMessages(ctx context.Context, start, end chain.Epoch) (int64, error) {
	return a.agg.CountOfBlockMessages(ctx, start, end)
}

func (a IndexAclImpl) GetEpoch(ctx context.Context, epoch chain.Epoch) (*londobell.EpochReply, error) {
	return a.adapter.Epoch(ctx, &epoch)
}

func (a IndexAclImpl) GetAggLatestTipset(ctx context.Context) (tipset *londobell.Tipset, err error) {
	
	tipsets, err := a.agg.LatestTipset(ctx)
	if err != nil {
		return
	}
	
	if len(tipsets) > 0 {
		tipset = tipsets[0]
	} else {
		err = fmt.Errorf("agg latest tipset is empty")
		return
	}
	
	return
}

//func (a IndexAclImpl) GetBaseFee(ctx context.Context) (baseFee *decimal.Decimal, err error) {
//	tipset, err := a.agg.FinalHeight(ctx)
//	if err != nil {
//		return
//	}
//	var latestTipset *londobell.Tipset
//	if tipset != nil {
//		latestTipset = tipset[0]
//	}
//	baseFee = &latestTipset.BaseFee
//	return
//}

func (a IndexAclImpl) GetPowerIncrease24H(ctx context.Context, epoch chain.Epoch) (powerIncrease24H decimal.Decimal, err error) {
	
	end := epoch - 2880
	startPowerState, err := a.GetTotalEpochPower(ctx, epoch)
	if err != nil {
		return
	}
	endPowerState, err := a.GetTotalEpochPower(ctx, end)
	if err != nil {
		return
	}
	powerIncrease24H = startPowerState.TotalQualityAdjPower.Sub(endPowerState.TotalQualityAdjPower)
	return
}

func (a IndexAclImpl) GetTotalEpochPower(ctx context.Context, epoch chain.Epoch) (powerDetail *londobell.PowerActorDetail, err error) {
	addr := chain.SmartAddress("04")
	actors, err := a.adapter.Actor(ctx, addr, &epoch)
	if err != nil {
		return
	}
	if actors == nil {
		err = fmt.Errorf("epoch: %d(%s) agg get f04 is emtpy", epoch.Int64(), epoch.Format())
		return
	}
	
	actorState, err := json.Marshal(actors.State)
	if err != nil {
		return
	}
	
	powerDetail = new(londobell.PowerActorDetail)
	err = json.Unmarshal(actorState, &powerDetail)
	if err != nil {
		return
	}
	return
}

func (a IndexAclImpl) GetRewardIncrease24H(ctx context.Context, epoch chain.Epoch) (decimal.Decimal, error) {
	end := epoch - 2880
	startRewardState, err := a.GetTotalEpochReward(ctx, epoch)
	if err != nil {
		return decimal.Zero, err
	}
	endRewardState, err := a.GetTotalEpochReward(ctx, end)
	if err != nil {
		return decimal.Zero, err
	}
	rewardIncrease24H := startRewardState.TotalStoragePowerReward.Sub(endRewardState.TotalStoragePowerReward)
	return rewardIncrease24H, err
}

func (a IndexAclImpl) GetTotalEpochReward(ctx context.Context, epoch chain.Epoch) (rewardDetail *londobell.RewardActorDetail, err error) {
	addr := chain.SmartAddress("02")
	actors, err := a.adapter.Actor(ctx, addr, &epoch)
	if err != nil {
		return
	}
	if actors == nil {
		err = fmt.Errorf("agg get actor f02 is empty")
		return
	}
	
	actorState, err := json.Marshal(actors.State)
	if err != nil {
		return
	}
	
	rewardDetail = new(londobell.RewardActorDetail)
	err = json.Unmarshal(actorState, &rewardDetail)
	if err != nil {
		return
	}
	
	return
}

func (a IndexAclImpl) GetWinCountReward(ctx context.Context, epoch chain.Epoch) (result decimal.Decimal, err error) {
	actorID := chain.SmartAddress("02")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return
		}
	}
	rewardState := londobell.RewardActorState{}
	err = json.Unmarshal(actorState, &rewardState)
	if err != nil {
		return
	}
	result = rewardState.ThisEpochReward.Div(decimal.NewFromInt(5))
	return
}

func (a IndexAclImpl) GetAvgBlockCount(ctx context.Context, epoch chain.Epoch) (result decimal.Decimal, err error) {
	var filters types.Filters
	endEpoch := epoch.CurrentHour()
	filters.End = &endEpoch
	startEpoch := chain.Epoch(filters.End.Int64() - 2880)
	filters.Start = &startEpoch
	blockList, err := a.agg.BlockHeader(ctx, filters)
	if err != nil {
		return
	}
	if blockList != nil {
		result = decimal.NewFromFloat(float64(len(blockList))).Div(decimal.NewFromFloat(2880))
	}
	return
}

func (a IndexAclImpl) GetAvgMessageCount(ctx context.Context, epoch chain.Epoch) (result int64, err error) {
	var filters types.Filters
	endEpoch := epoch.CurrentHour()
	filters.End = &endEpoch
	startEpoch := chain.Epoch(filters.End.Int64() - 2880)
	filters.Start = &startEpoch
	blockList, err := a.agg.BlockHeader(ctx, filters)
	if err != nil {
		return
	}
	if blockList != nil {
		var sumMessageCount int64
		for _, block := range blockList {
			sumMessageCount = sumMessageCount + block.MessageCount
		}
		result = sumMessageCount / 2880
	}
	return
}

type NetPower struct {
	QualityPower decimal.Decimal
	RawBytePower decimal.Decimal
}

func (a IndexAclImpl) GetTotalQualityPower(ctx context.Context, epoch chain.Epoch) (power NetPower, err error) {
	actorID := chain.SmartAddress("04")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return
		}
	}
	powerState := londobell.PowerActorState{}
	err = json.Unmarshal(actorState, &powerState)
	if err != nil {
		return
	}
	power.QualityPower = powerState.TotalQualityAdjPower
	power.RawBytePower = powerState.TotalRawBytePower
	return
}

func (a IndexAclImpl) GetTotalRewards(ctx context.Context, epoch chain.Epoch) (totalRewards decimal.Decimal, err error) {
	actorID := chain.SmartAddress("02")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return
		}
	}
	rewardState := londobell.RewardActorState{}
	err = json.Unmarshal(actorState, &rewardState)
	if err != nil {
		return
	}
	totalRewards = rewardState.TotalStoragePowerReward
	return
}

func (a IndexAclImpl) GetActiveMiners(ctx context.Context, epoch chain.Epoch) (count int64, err error) {
	actorID := chain.SmartAddress("04")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return
		}
	}
	powerState := londobell.PowerActorState{}
	err = json.Unmarshal(actorState, &powerState)
	if err != nil {
		return
	}
	count = powerState.MinerAboveMinPowerCount
	return
}

func (a IndexAclImpl) GetBurnt(ctx context.Context, epoch chain.Epoch) (burnt decimal.Decimal, err error) {
	actorID := chain.SmartAddress("099")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	burnt = actor.Balance
	return
}

func (a IndexAclImpl) GetMinerSectorSize(ctx context.Context, miner chain.SmartAddress) (sectorSize decimal.Decimal, err error) {
	var epoch *chain.Epoch
	actor, err := a.adapter.Miner(ctx, miner, epoch)
	if err != nil {
		return
	}
	var actorSectorSize int64
	if actor != nil {
		actorSectorSize = actor.SectorSize
	}
	decimalSectorSize := decimal.NewFromInt(actorSectorSize)
	sectorSize = decimalSectorSize
	return
}

func (a IndexAclImpl) GetInitialPledge(ctx context.Context) (initialPledge decimal.Decimal, err error) {
	sector, err := a.adapter.CurrentSectorInitialPledge(ctx, nil)
	if err != nil {
		return
	}
	var currentSectorInitialPledge decimal.Decimal
	if sector != nil {
		currentSectorInitialPledge = sector.CurrentSectorInitialPledge
	}
	decimalInitialPledge := currentSectorInitialPledge
	initialPledge = decimalInitialPledge
	return
}

func (a IndexAclImpl) GetCirculatingPercent(ctx context.Context) (circulatingPercent decimal.Decimal, err error) {
	sector, err := a.adapter.CurrentSectorInitialPledge(ctx, nil)
	if err != nil {
		return
	}
	var circulatingRate decimal.Decimal
	if sector != nil {
		circulatingRate = sector.CirculatingRate
	}
	circulatingPercent = circulatingRate
	return
}
