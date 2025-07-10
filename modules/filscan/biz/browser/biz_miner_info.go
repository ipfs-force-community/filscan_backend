package browser

import (
	"context"
	"fmt"
	"math/big"

	"github.com/gozelle/pointer"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/debuglog"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/gorm"
)

func NewMinerInfoBiz(db *gorm.DB, agg londobell.Agg, adapter londobell.Adapter) *MinerInfoBiz {
	return &MinerInfoBiz{
		db:      db,
		se:      dal.NewSyncEpochGetterDal(db),
		sr:      dal.NewSyncerDal(db),
		adapter: adapter,
		agg:     agg,
	}
}

type MinerInfoBiz struct {
	db      *gorm.DB
	se      repository.SyncEpochGetter
	sr      repository.SyncerRepo
	adapter londobell.Adapter
	agg     londobell.Agg
}

func (m MinerInfoBiz) CheckIsMiner(ctx context.Context, addr chain.SmartAddress) (result bool, err error) {
	checker := dal.NewOwnerGetterDal(m.db)
	ok, err := checker.IsOwner(ctx, addr)
	if err != nil {
		return
	}
	result = ok
	return
}

func (m MinerInfoBiz) GetMinerInfo(ctx context.Context, addr chain.SmartAddress) (result *filscan.AccountMiner, err error) {
	minerInfoDal := dal.NewMinerInfoBizDal(m.db)
	minerInfo, err := minerInfoDal.GetMinerInfo(ctx, addr)
	if err != nil {
		return
	}
	if minerInfo != nil {
		convert := assembler.MinerInfoAssembler{}
		var newMinerInfo *filscan.AccountMiner
		newMinerInfo, err = convert.ToMinerInfoResponse(minerInfo)
		if err != nil {
			return
		}
		result = newMinerInfo
	}
	return
}

func (m MinerInfoBiz) GetMinerIpAddress(ctx context.Context, addr chain.SmartAddress) (address string, err error) {

	defer func() {
		if address == "" {
			addr = "-"
		}
	}()

	minerInfoDal := dal.NewMinerInfoBizDal(m.db)
	loc, err := minerInfoDal.GetMinerAddressOrNil(ctx, addr)
	if err != nil {
		return
	}
	if loc == nil || loc.City == nil {
		return
	}

	address = *loc.City

	return
}

func (m MinerInfoBiz) GetMinerIndicator(ctx context.Context, addr chain.SmartAddress, interval *types.IntervalType) (result *filscan.MinerIndicators, err error) {
	defer func() {
		debuglog.Logger.Info("result", result, err, addr)
	}()

	tipset, err := m.agg.LatestTipset(ctx)
	if err != nil {
		return nil, err
	}
	var filters types.Filters
	if tipset != nil {
		endEpoch := chain.Epoch(tipset[0].ID)
		filters.End = &endEpoch
	}

	endMinerInfo, err := m.adapter.Miner(ctx, addr, filters.End)
	if err != nil {
		return
	}

	minerInfoDal := dal.NewMinerInfoBizDal(m.db)
	var startMinerInfo *bo.MinerInfo
	var day decimal.Decimal
	switch interval.Value() {
	case types.DAY:
		startEpoch := chain.Epoch(filters.End.Int64() - 2880)
		filters.Start = &startEpoch
		var startMiner *londobell.MinerDetail
		startMiner, err = m.adapter.Miner(ctx, addr, &startEpoch)
		if err != nil {
			return
		}
		startMinerInfo = &bo.MinerInfo{
			InitialPledge:   startMiner.State.InitialPledge,
			QualityAdjPower: startMiner.QualityPower,
			SectorSize:      startMiner.SectorSize,
			SectorCount:     startMiner.SectorCount,
		}
		day = decimal.NewFromInt(1)
	case types.WEEK:
		startEpoch := chain.Epoch(filters.End.Int64() - 2880*7)
		filters.Start = &startEpoch
		startMinerInfo, err = minerInfoDal.GetMinerInfoByEpoch(ctx, addr, startEpoch)
		if err != nil {
			return
		}
		day = decimal.NewFromInt(7)
	case types.MONTH:
		startEpoch := chain.Epoch(filters.End.Int64() - 2880*30)
		filters.Start = &startEpoch
		startMinerInfo, err = minerInfoDal.GetMinerInfoByEpoch(ctx, addr, startEpoch)
		if err != nil {
			return
		}
		day = decimal.NewFromInt(30)
	case types.YEAR:
		startEpoch := chain.Epoch(filters.End.Int64() - 2880*365)
		filters.Start = &startEpoch
		startMinerInfo, err = minerInfoDal.GetMinerInfoByEpoch(ctx, addr, startEpoch)
		if err != nil {
			return
		}
		day = decimal.NewFromInt(365)
	default:
		err = fmt.Errorf("unkonwn interval type: %s", interval.Value())
		return
	}
	var powerIncrease decimal.Decimal
	var powerRatio decimal.Decimal
	var sectorIncrease decimal.Decimal
	var sectorRatio decimal.Decimal
	var sectorDeposits decimal.Decimal
	if startMinerInfo != nil {
		powerIncrease = endMinerInfo.QualityPower.Sub(startMinerInfo.QualityAdjPower)
		powerRatio = endMinerInfo.QualityPower.Sub(startMinerInfo.QualityAdjPower).Div(day)
		sectorIncrease = decimal.NewFromInt((endMinerInfo.SectorCount - startMinerInfo.SectorCount) * endMinerInfo.SectorSize)
		sectorRatio = sectorIncrease.Div(day)
		sectorDeposits = endMinerInfo.InitialPledgeRequirement.Sub(startMinerInfo.InitialPledge)
	}

	var luckyRate decimal.Decimal
	minerIndicatorDal := dal.NewMinerIndicatorDal(m.db)
	lucky, err := minerIndicatorDal.GetMinerLucky(ctx, addr, interval)
	if err != nil {
		return
	}
	//if minerIndicator != nil {
	//	luckyRate = minerIndicator.LuckRate
	//}
	luckyRate = lucky
	accIndicators, err := minerIndicatorDal.GetMinerAccIndicators(ctx, addr, interval)
	if err != nil {
		return
	}
	var totalBlockCount int64
	var totalBlockRewards decimal.Decimal
	var totalWinCount int64
	var totalGasCost decimal.Decimal
	var gasFeePerTB decimal.Decimal
	var rewardsPerTB decimal.Decimal
	var windowPoStGas decimal.Decimal
	var windowPoStGasPerTB decimal.Decimal
	if accIndicators != nil {
		totalBlockCount = accIndicators.AccBlockCount
		totalBlockRewards = accIndicators.AccReward
		totalWinCount = accIndicators.AccWinCount
		totalGasCost = accIndicators.AccSealGas.Add(accIndicators.AccWdPostGas)
		windowPoStGas = accIndicators.AccWdPostGas
		if sectorIncrease.IsZero() {
			gasFeePerTB = decimal.NewFromInt(0)
		} else {
			gasFeePerTB = accIndicators.AccSealGas.Div(sectorIncrease.Div(chain.PerT))
		}
		if endMinerInfo.QualityPower.IsZero() {
			rewardsPerTB = decimal.NewFromInt(0)
		} else {
			rewardsPerTB = totalBlockRewards.Div(endMinerInfo.QualityPower.Div(chain.PerT))
		}
		if endMinerInfo.Power.IsZero() {
			windowPoStGasPerTB = decimal.NewFromInt(0)
		} else {
			windowPoStGasPerTB = windowPoStGas.Div(endMinerInfo.Power.Div(chain.PerT))
		}
	} else {
		log.Warnf("fail to get acc indicators of miner %s", addr)
	}

	newMinerIndicator := filscan.MinerIndicators{
		PowerIncrease:       powerIncrease,
		PowerRatio:          powerRatio,
		SectorIncrease:      sectorIncrease,
		SectorRatio:         sectorRatio,
		SectorDeposits:      sectorDeposits,
		GasFee:              totalGasCost,
		BlockCountIncrease:  totalBlockCount,
		BlockRewardIncrease: totalBlockRewards,
		WinCount:            totalWinCount,
		RewardsPerTB:        rewardsPerTB,
		GasFeePerTB:         gasFeePerTB,
		Lucky:               luckyRate,
		WindowPoStGas:       windowPoStGasPerTB,
	}
	result = &newMinerIndicator

	return
}

func (m MinerInfoBiz) MinerEpoch(ctx context.Context) (epoch *int64, err error) {
	r, err := m.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	if r == nil {
		return
	}

	epoch = pointer.ToInt64(r.Int64())

	return
}

func (m MinerInfoBiz) MinerBalanceTrend(ctx context.Context, accountID chain.SmartAddress, accountInterval types.IntervalType, epoch *int64, balances []*filscan.BalanceTrend) (balanceTrends []*filscan.BalanceTrend, err error) {
	if epoch == nil {
		return
	}
	var resolveInterval interval.Interval
	resolveInterval, err = interval.ResolveInterval(string(accountInterval), chain.Epoch(*epoch))
	if err != nil {
		return
	}
	minerBalanceDal := dal.NewMinerBalanceTrendBizDal(m.db)
	var minerBalanceTrend []*bo.ActorBalanceTrend
	minerBalanceTrend, err = minerBalanceDal.GetMinerBalanceTrend(ctx, resolveInterval.Points(), accountID)
	if err != nil {
		return
	}
	convert := assembler.MinerInfoAssembler{}
	var newMinerBalanceTrends []*filscan.BalanceTrend
	newMinerBalanceTrends, err = convert.ToMinerBalanceTrendResponse(minerBalanceTrend)
	if err != nil {
		return
	}
	balanceTrends = fixMinerBalanceTrends(newMinerBalanceTrends, resolveInterval.Points(), balances)

	return
}

func (m MinerInfoBiz) MinerPowerTrend(ctx context.Context, accountID chain.SmartAddress, accountInterval types.IntervalType) (powerTrends []*filscan.PowerTrend, err error) {
	epoch, err := m.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	if epoch == nil {
		return
	}
	var resolveInterval interval.Interval
	resolveInterval, err = interval.ResolveInterval(string(accountInterval), *epoch)
	if err != nil {
		return
	}

	minerPowerDal := dal.NewMinerPowerTrendBizDal(m.db)
	var minerPowerTrend []*bo.ActorPowerTrend
	minerPowerTrend, err = minerPowerDal.GetMinerPowerTrend(ctx, resolveInterval.Points(), accountID)
	if err != nil {
		return
	}
	convert := assembler.MinerInfoAssembler{}
	var newMinerPowerTrends []*filscan.PowerTrend
	newMinerPowerTrends, err = convert.ToMinerPowerTrendResponse(minerPowerTrend)
	if err != nil {
		return
	}
	powerTrends = fixPowerTrends(newMinerPowerTrends, resolveInterval.Points())

	return
}

func fixMinerBalanceTrends(trends []*filscan.BalanceTrend, points []chain.Epoch, balances []*filscan.BalanceTrend) (result []*filscan.BalanceTrend) {

	maxEpoch := int64(0)
	trendMap := map[int64]*filscan.BalanceTrend{}
	for _, v := range trends {
		trendMap[v.Epoch] = v
		if v.Epoch > maxEpoch {
			maxEpoch = v.Epoch
		}
	}
	balanceMap := map[int64]decimal.Decimal{}
	for _, v := range balances {
		balanceMap[v.Epoch] = v.Balance
	}
	for i := len(points) - 1; i >= 0; i-- {
		v := points[i]
		if vv, ok := trendMap[v.Int64()]; ok {
			result = append(result, vv)
			continue
		}
		if v.Int64() > maxEpoch {
			b := balanceMap[v.Int64()]
			result = append(result, &filscan.BalanceTrend{
				Height:            big.NewInt(v.Int64()),
				BlockTime:         v.Unix(),
				Balance:           b,
				AvailableBalance:  &b,
				InitialPledge:     &decimal.Zero,
				LockedFunds:       &decimal.Zero,
				PreCommitDeposits: &decimal.Zero,
				Epoch:             v.Int64(),
			})
		}
	}

	return
}

func fixPowerTrends(trends []*filscan.PowerTrend, points []chain.Epoch) (result []*filscan.PowerTrend) {

	maxEpoch := int64(0)
	trendMap := map[int64]*filscan.PowerTrend{}
	for _, v := range trends {
		trendMap[v.Epoch] = v
		if v.Epoch > maxEpoch {
			maxEpoch = v.Epoch
		}
	}

	for i := len(points) - 1; i >= 0; i-- {
		v := points[i]
		if vv, ok := trendMap[v.Int64()]; ok {
			result = append(result, vv)
			continue
		}
		if v.Int64() > maxEpoch {
			result = append(result, &filscan.PowerTrend{
				BlockTime:     v.Unix(),
				Power:         decimal.Zero,
				PowerIncrease: decimal.Zero,
				Epoch:         v.Int64(),
			})
		}
	}

	return
}
