package probiz

import (
	"context"
	"time"

	"github.com/golang-module/carbon/v2"
	"github.com/gozelle/async/collection"
	"github.com/shopspring/decimal"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	probo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/bo"
	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger"
	mergerimpl "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger/impl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/vip"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

func NewMiner(db *gorm.DB, adapter londobell.Adapter, agg londobell.Agg, minerAgg londobell.MinerAgg) *MinerBiz {
	return &MinerBiz{
		db:             db,
		userMinersRepo: prodal.NewUserMinerDal(db),
		merger:         mergerimpl.NewMergerImpl(db, adapter, agg, minerAgg),
	}
}

var _ pro.MinerAPI = (*MinerBiz)(nil)

type MinerBiz struct {
	db             *gorm.DB
	userMinersRepo prorepo.UserMinerRepo
	groupRepo      prorepo.GroupRepo
	merger         merger.Merger
}

func (m MinerBiz) MinerInfoDetail(ctx context.Context, req pro.MinerInfoDetailRequest) (response pro.MinerInfoDetailResponse, err error) {
	groupMiners, err := m.checkInputGroupMiners(ctx, req.GroupID, nil)
	if err != nil {
		return
	}
	date, err := chain.NewDate(carbon.Shanghai, time.Now().Format(carbon.DateLayout))
	if err != nil {
		return
	}
	minerIDList, minerInfo := probo.ConvertGroupMiners(groupMiners)

	var minersInfo map[chain.SmartAddress]*merger.MinerInfo
	var updated chain.Epoch
	var summary merger.MinersSummary
	updated, summary, minersInfo, err = m.merger.MinersInfos(ctx, minerIDList, date)
	if err != nil {
		return
	}

	var newMinerInfoDetailList []*pro.MinerInfoDetail
	for _, minerID := range minerIDList {
		item := &pro.MinerInfoDetail{
			Tag:                  minerInfo[minerID].MinerTag,
			MinerId:              minerID,
			GroupName:            minerInfo[minerID].GroupName,
			IsDefault:            minerInfo[minerID].IsDefault,
			TotalQualityAdjPower: decimal.Decimal{},
			TotalRawBytePower:    decimal.Decimal{},
			TotalReward:          decimal.Decimal{},
			RewardChange24h:      decimal.Decimal{},
			TotalOutlay:          decimal.Decimal{},
			TotalGas:             decimal.Decimal{},
			TotalPledge:          decimal.Decimal{},
			PledgeChange24h:      decimal.Decimal{},
			TotalBalance:         decimal.Decimal{},
			BalanceChange24h:     decimal.Decimal{},
		}

		if vv, ok := minersInfo[minerID]; ok {
			item.TotalQualityAdjPower = decimal.Decimal(vv.QualityAdjPower)
			item.TotalRawBytePower = decimal.Decimal(vv.RawBytePower)
			item.TotalReward = decimal.Decimal(vv.Reward)
			item.RewardChange24h = decimal.Decimal(vv.RewardZero)
			item.TotalOutlay = decimal.Decimal(vv.Outlay)
			item.TotalGas = decimal.Decimal(vv.Gas)
			item.TotalPledge = decimal.Decimal(vv.PledgeAmount)
			item.PledgeChange24h = decimal.Decimal(vv.PledgeZero)
			item.TotalBalance = decimal.Decimal(vv.Balance)
			item.BalanceChange24h = decimal.Decimal(vv.BalanceZero)
		}
		newMinerInfoDetailList = append(newMinerInfoDetailList, item)
	}

	collection.Sort[*pro.MinerInfoDetail](newMinerInfoDetailList, func(a, b *pro.MinerInfoDetail) bool {
		return a.MinerId < b.MinerId
	})

	response = pro.MinerInfoDetailResponse{
		Epoch:               updated,
		EpochTime:           updated.Time(),
		SumQualityAdjPower:  summary.TotalQualityAdjPower.Decimal(),
		SumPowerChange24h:   summary.TotalQualityAdjPowerZero.Decimal(),
		SumReward:           summary.TotalReward.Decimal(),
		SumRewardChange24h:  summary.TotalRewardZero.Decimal(),
		SumOutlay:           summary.TotalOutcome.Decimal(),
		SumGas:              summary.TotalGas.Decimal(),
		SumPledge:           summary.TotalPledge.Decimal(),
		SumPledgeChange24h:  summary.TotalPledgeZero.Decimal(),
		SumBalance:          summary.TotalBalance.Decimal(),
		SumBalanceChange24h: summary.TotalBalanceZero.Decimal(),
		MinerInfoDetailList: newMinerInfoDetailList,
	}
	return
}

func (m MinerBiz) PowerDetail(ctx context.Context, req pro.PowerDetailRequest) (response pro.PowerDetailResponse, err error) {
	groupMiners, err := m.checkInputGroupMiners(ctx, req.GroupID, req.MinerID)
	if err != nil {
		return
	}
	dates, err := m.checkInputDate(req.StartDate, req.EndDate)
	if err != nil {
		return
	}
	minerIDList, minerInfo := probo.ConvertGroupMiners(groupMiners)
	var minersPower []*merger.DayPowerStat
	var updated chain.Epoch
	updated, minersPower, err = m.merger.MinersPowerStats(ctx, minerIDList, dates)
	if err != nil {
		return
	}

	for _, minerID := range minerIDList {
		for _, power := range minersPower {
			item := &pro.PowerDetail{
				Date:              power.Day.Time(),
				Tag:               minerInfo[minerID].MinerTag,
				MinerId:           minerID,
				GroupName:         minerInfo[minerID].GroupName,
				IsDefault:         minerInfo[minerID].IsDefault,
				QualityPower:      decimal.Decimal{},
				RawPower:          decimal.Decimal{},
				DCPower:           decimal.Decimal{},
				CCPower:           decimal.Decimal{},
				SectorSize:        decimal.Decimal{},
				SectorPowerChange: decimal.Decimal{},
				SectorCountChange: 0,
				PledgeChanged:     decimal.Decimal{},
				PledgeChangedPerT: decimal.Decimal{},
				Penalty:           decimal.Decimal{},
				FaultSectors:      0,
			}
			if vv, ok := power.Stats[minerID]; ok {
				item.QualityPower = vv.QualityAdjPower.Decimal()
				item.RawPower = vv.RawBytePower.Decimal()
				item.DCPower = vv.VdcPower.Decimal()
				item.CCPower = vv.CcPower.Decimal()
				item.SectorSize = vv.SectorSize.Decimal()
				item.SectorPowerChange = vv.TotalSectorsPowerZero.Decimal()
				item.SectorCountChange = vv.TotalSectorsZero
				item.PledgeChanged = vv.PledgeAmountZero.Decimal()
				item.PledgeChangedPerT = vv.PledgeAmountZeroPert.Decimal()
				item.Penalty = vv.PenaltyZero.Decimal()
				item.FaultSectors = vv.FaultSectors
			}
			response.PowerDetailList = append(response.PowerDetailList, item)
		}
	}

	collection.Sort[*pro.PowerDetail](response.PowerDetailList, func(a, b *pro.PowerDetail) bool {
		return a.MinerId < b.MinerId
	})

	response.Epoch = updated
	response.EpochTime = updated.Time()
	return
}

func (m MinerBiz) GasCostDetail(ctx context.Context, req pro.GasCostDetailRequest) (response pro.GasCostDetailResponse, err error) {
	groupMiners, err := m.checkInputGroupMiners(ctx, req.GroupID, req.MinerID)
	if err != nil {
		return
	}
	dates, err := m.checkInputDate(req.StartDate, req.EndDate)
	if err != nil {
		return
	}

	minerIDList, minerInfo := probo.ConvertGroupMiners(groupMiners)
	var minersGasCost []*merger.DayFundStat
	var updated chain.Epoch

	updated, minersGasCost, err = m.merger.MinersFundStats(ctx, minerIDList, dates)
	if err != nil {
		return
	}
	all := &pro.GasCostDetail{
		Date: "All",
		Tag:  "-",
	}
	for _, minerID := range minerIDList {
		for _, power := range minersGasCost {
			all.MinerId = minerID
			all.SectorCountChange += power.Stats[minerID].TotalSectorsZero
			all.SectorPowerChange = power.Stats[minerID].TotalSectorsPowerZero.Decimal().Add(all.SectorPowerChange)
			all.TotalGasCost = power.Stats[minerID].TotalGas.Decimal().Add(all.TotalGasCost)
			all.SealGasCost = power.Stats[minerID].SealGas.Decimal().Add(all.SealGasCost)
			all.DealGasCost = power.Stats[minerID].PublishDealGas.Decimal().Add(all.DealGasCost)
			all.WdPostGasCost = power.Stats[minerID].WdPostGas.Decimal().Add(all.WdPostGasCost)

			response.GasCostDetailList = append(response.GasCostDetailList, &pro.GasCostDetail{
				Date:              power.Day.Time().String(),
				Tag:               minerInfo[minerID].MinerTag,
				MinerId:           minerID,
				GroupName:         minerInfo[minerID].GroupName,
				IsDefault:         minerInfo[minerID].IsDefault,
				SectorPowerChange: power.Stats[minerID].TotalSectorsPowerZero.Decimal(),
				SectorCountChange: power.Stats[minerID].TotalSectorsZero,
				TotalGasCost:      power.Stats[minerID].TotalGas.Decimal(),
				SealGasCost:       power.Stats[minerID].SealGas.Decimal(),
				SealGasPerT:       power.Stats[minerID].SealGasPerT.Decimal(),
				DealGasCost:       power.Stats[minerID].PublishDealGas.Decimal(),
				WdPostGasCost:     power.Stats[minerID].WdPostGas.Decimal(),
				WdPostGasPerT:     power.Stats[minerID].WdPostGasPerT.Decimal(),
			})
		}
	}
	collection.Sort[*pro.GasCostDetail](response.GasCostDetailList, func(a, b *pro.GasCostDetail) bool {
		return a.MinerId < b.MinerId
	})

	if req.MinerID != nil {
		response.GasCostDetailList = append([]*pro.GasCostDetail{all}, response.GasCostDetailList...)
	}
	response.Epoch = updated
	response.EpochTime = updated.Time()
	return
}

func (m MinerBiz) SectorDetail(ctx context.Context, req pro.SectorDetailRequest) (response pro.SectorDetailResponse, err error) {
	groupMiners, err := m.checkInputGroupMiners(ctx, req.GroupID, req.MinerID)
	if err != nil {
		return
	}
	minerIDList, minerInfo := probo.ConvertGroupMiners(groupMiners)

	var minersSectors *merger.SectorStat
	var updated chain.Epoch
	updated, minersSectors, err = m.merger.MinersSectorStats(ctx, minerIDList)
	if err != nil {
		return
	}
	if req.MinerID == nil {
		for _, sector := range minersSectors.Months {
			var newMinerSectorDetail []*pro.SectorDetail
			for _, miner := range sector.Miners {
				newMinerSectorDetail = append(newMinerSectorDetail, &pro.SectorDetail{
					Tag:            minerInfo[miner.Miner].MinerTag,
					MinerId:        miner.Miner,
					GroupName:      minerInfo[miner.Miner].GroupName,
					IsDefault:      minerInfo[miner.Miner].IsDefault,
					ExpPower:       miner.Power.Decimal(),
					ExpSectorCount: miner.Sectors,
					ExpDC:          miner.VDC.Decimal(),
					ExpPledge:      miner.Pledge.Decimal(),
				})
			}
			response.SectorDetailMonth = append(response.SectorDetailMonth, &pro.SectorDetailMonth{
				ExpMonth:            sector.Month,
				TotalMinerCount:     int64(len(sector.Miners)),
				TotalExpPower:       sector.MinerSectorStat.Power.Decimal(),
				TotalExpSectorCount: sector.MinerSectorStat.Sectors,
				TotalExpDC:          sector.MinerSectorStat.VDC.Decimal(),
				TotalExpPledge:      sector.MinerSectorStat.Pledge.Decimal(),
				SectorDetailList:    newMinerSectorDetail,
			})
			response.Summary.TotalPower = response.Summary.TotalPower.Add(sector.Power.Decimal())
			response.Summary.TotalDc = response.Summary.TotalDc.Add(sector.VDC.Decimal())
			response.Summary.TotalCC = response.Summary.TotalCC.Add(sector.CC.Decimal())
		}
	} else {
		for _, sector := range minersSectors.Days {
			for _, miner := range sector.Miners {
				response.SectorDetailDay = append(response.SectorDetailDay, &pro.SectorDetail{
					ExpDate:        sector.Day,
					Tag:            minerInfo[miner.Miner].MinerTag,
					MinerId:        miner.Miner,
					GroupName:      minerInfo[miner.Miner].GroupName,
					IsDefault:      minerInfo[miner.Miner].IsDefault,
					ExpPower:       miner.Power.Decimal(),
					ExpSectorCount: miner.Sectors,
					ExpDC:          miner.VDC.Decimal(),
					ExpPledge:      miner.Pledge.Decimal(),
				})
			}
			response.Summary.TotalPower = response.Summary.TotalPower.Add(sector.Power.Decimal())
			response.Summary.TotalDc = response.Summary.TotalDc.Add(sector.VDC.Decimal())
			response.Summary.TotalCC = response.Summary.TotalCC.Add(sector.CC.Decimal())
		}
	}

	response.Epoch = updated
	response.EpochTime = updated.Time()
	return
}

func (m MinerBiz) RewardDetail(ctx context.Context, req pro.RewardDetailRequest) (response pro.RewardDetailResponse, err error) {

	groupMiners, err := m.checkInputGroupMiners(ctx, req.GroupID, req.MinerID)
	if err != nil {
		return
	}
	dates, err := m.checkInputDate(req.StartDate, req.EndDate)
	if err != nil {
		return
	}
	minerIDList, minerInfo := probo.ConvertGroupMiners(groupMiners)

	var minersReward []*merger.DayRewardStat
	var updated chain.Epoch
	updated, minersReward, err = m.merger.MinersRewardStats(ctx, minerIDList, dates)
	if err != nil {
		return
	}
	all := &pro.RewardDetail{
		Date: "All",
	}
	for _, minerID := range minerIDList {
		for _, reward := range minersReward {
			all.MinerId = minerID
			all.BlockCount += reward.Stats[minerID].Blocks
			all.WinCount += reward.Stats[minerID].WinCounts
			all.BlockReward = reward.Stats[minerID].Rewards.Decimal().Add(all.BlockReward)
			all.TotalReward = reward.Stats[minerID].TotalRewards.Decimal().Add(all.BlockReward)
			response.RewardDetailList = append(response.RewardDetailList, &pro.RewardDetail{
				Date:        reward.Day.Time().String(),
				Tag:         minerInfo[minerID].MinerTag,
				MinerId:     minerID,
				GroupName:   minerInfo[minerID].GroupName,
				IsDefault:   minerInfo[minerID].IsDefault,
				BlockCount:  reward.Stats[minerID].Blocks,
				WinCount:    reward.Stats[minerID].WinCounts,
				BlockReward: reward.Stats[minerID].Rewards.Decimal(),
				TotalReward: reward.Stats[minerID].TotalRewards.Decimal(),
			})
		}
	}
	collection.Sort[*pro.RewardDetail](response.RewardDetailList, func(a, b *pro.RewardDetail) bool {
		return a.MinerId < b.MinerId
	})

	if req.MinerID != nil {
		response.RewardDetailList = append([]*pro.RewardDetail{all}, response.RewardDetailList...)
	}
	response.Epoch = updated
	response.EpochTime = updated.Time()
	return
}

func (m MinerBiz) LuckyRateDetail(ctx context.Context, req pro.LuckyRateDetailRequest) (response pro.LuckyRateDetailResponse, err error) {
	groupMiners, err := m.checkInputGroupMiners(ctx, req.GroupID, req.MinerID)
	if err != nil {
		return
	}
	minerIDList, minerInfo := probo.ConvertGroupMiners(groupMiners)
	var minersLuck map[chain.SmartAddress]*merger.LuckStats
	var updated chain.Epoch
	updated, minersLuck, err = m.merger.MinersLuckStats(ctx, minerIDList)
	if err != nil {
		return
	}

	for _, minerID := range minerIDList {
		response.LuckyRateList = append(response.LuckyRateList, &pro.LuckyRateDetail{
			Tag:          minerInfo[minerID].MinerTag,
			MinerID:      minerID,
			GroupName:    minerInfo[minerID].GroupName,
			IsDefault:    minerInfo[minerID].IsDefault,
			LuckyRate24h: minersLuck[minerID].Luck24h,
			LuckyRate7d:  minersLuck[minerID].Luck7d,
			LuckyRate30d: minersLuck[minerID].Luck30d,
		})
	}
	collection.Sort[*pro.LuckyRateDetail](response.LuckyRateList, func(a, b *pro.LuckyRateDetail) bool {
		return a.MinerID < b.MinerID
	})
	response.Epoch = updated
	response.EpochTime = updated.Time()
	return
}

func (m MinerBiz) BalanceDetail(ctx context.Context, req pro.BalanceDetailRequest) (response pro.BalanceDetailResponse, err error) {
	groupMiners, err := m.checkInputGroupMiners(ctx, req.GroupID, req.MinerID)
	if err != nil {
		return
	}

	date, err := chain.NewDate(carbon.Shanghai, time.Now().Format(carbon.DateLayout))
	if err != nil {
		return
	}
	minerIDList, minerInfo := probo.ConvertGroupMiners(groupMiners)
	var minersBalance map[chain.SmartAddress]*merger.BalanceStat
	var updated chain.Epoch
	updated, minersBalance, err = m.merger.MinersBalanceStats(ctx, minerIDList, date)
	if err != nil {
		return
	}

	for _, minerID := range minerIDList {
		response.BalanceDetailList = append(response.BalanceDetailList, &pro.BalanceDetail{
			Tag:                       minerInfo[minerID].MinerTag,
			MinerID:                   minerID,
			GroupName:                 minerInfo[minerID].GroupName,
			IsDefault:                 minerInfo[minerID].IsDefault,
			MinerBalance:              minersBalance[minerID].Miner.Decimal(),
			MinerBalanceChanged:       minersBalance[minerID].MinerZero.Decimal(),
			OwnerBalance:              minersBalance[minerID].Owner.Decimal(),
			OwnerBalanceChanged:       minersBalance[minerID].OwnerZero.Decimal(),
			WorkerBalance:             minersBalance[minerID].Worker.Decimal(),
			WorkerBalanceChanged:      minersBalance[minerID].WorkerZero.Decimal(),
			Controller0Balance:        minersBalance[minerID].C0.Decimal(),
			Controller0BalanceChanged: minersBalance[minerID].C0Zero.Decimal(),
			Controller1Balance:        minersBalance[minerID].C1.Decimal(),
			Controller1BalanceChanged: minersBalance[minerID].C1Zero.Decimal(),
			Controller2Balance:        minersBalance[minerID].C2.Decimal(),
			Controller2BalanceChanged: minersBalance[minerID].C2Zero.Decimal(),
			BeneficiaryBalance:        minersBalance[minerID].Beneficiary.Decimal(),
			BeneficiaryBalanceChanged: minersBalance[minerID].BeneficiaryZero.Decimal(),
			MarketBalance:             minersBalance[minerID].Market.Decimal(),
			MarketBalanceChanged:      minersBalance[minerID].MarketZero.Decimal(),
		})
	}
	collection.Sort[*pro.BalanceDetail](response.BalanceDetailList, func(a, b *pro.BalanceDetail) bool {
		return a.MinerID < b.MinerID
	})
	response.Epoch = updated
	response.EpochTime = updated.Time()
	return
}

func (m MinerBiz) checkInputGroupMiners(ctx context.Context, groupID int64, minerID *chain.SmartAddress) (groupMiners []*probo.GroupMiners, err error) {
	b := bearer.UseBearer(ctx)
	v := vip.UseVIP(ctx)
	maxMinersCount := VipMaxMinersCount(v.MType)
	currentUserMiners, err := m.userMinersRepo.SelectGroupMinersByUserID(ctx, b.Id, maxMinersCount)
	if err != nil {
		return
	}
	defer func() {
		// 修正默认分组
		for _, v := range groupMiners {
			if v.GroupID == 0 {
				v.IsDefault = true
			}
		}
	}()
	if minerID != nil {
		for _, groupMiner := range currentUserMiners {
			if groupMiner.MinerID == *minerID {
				groupMiners = append(groupMiners, &probo.GroupMiners{
					GroupID:   groupMiner.GroupID,
					GroupName: groupMiner.GroupName,
					IsDefault: groupMiner.IsDefault,
					MinersID: []*probo.MinerInfo{
						{
							MinerID:  groupMiner.MinerID,
							MinerTag: groupMiner.MinerTag,
						},
					},
				})
				break
			}
		}
	} else {
		if groupID == -1 {
			groupMiners = probo.UserMinersToGroupMiners(currentUserMiners)
		} else {
			var currentGroupMiners []*probo.UserMiner
			currentGroupMiners, err = m.userMinersRepo.SelectGroupMinersByGroupID(ctx, b.Id, groupID)
			if err != nil {
				return
			}
			groupMiners = probo.UserMinersToGroupMiners(currentGroupMiners)
			return
		}
	}
	return
}

func (m MinerBiz) checkInputDate(inputStartDate *string, inputEndDate *string) (dates chain.DateLCRCRange, err error) {
	var startDate chain.Date
	if inputStartDate == nil {
		startDate, err = chain.NewDate(carbon.Shanghai, time.Now().In(chain.TimeLoc).Format(carbon.DateLayout))
		if err != nil {
			return
		}
	} else {
		var startTime time.Time
		startTime, err = time.Parse(carbon.ISO8601Layout, *inputStartDate)
		if err != nil {
			return
		}
		startDate, err = chain.NewDate(carbon.Shanghai, startTime.Format(carbon.DateLayout))
		if err != nil {
			return
		}
	}
	var endDate chain.Date
	if inputEndDate == nil {
		endDate, err = chain.NewDate(carbon.Shanghai, time.Now().In(chain.TimeLoc).Format(carbon.DateLayout))
		if err != nil {
			return
		}
	} else {
		var endTime time.Time
		endTime, err = time.Parse(carbon.ISO8601Layout, *inputEndDate)
		if err != nil {
			return
		}
		endDate, err = chain.NewDate(carbon.Shanghai, endTime.Format(carbon.DateLayout))
		if err != nil {
			return
		}
	}

	dates = chain.DateLCRCRange{
		GteBegin: startDate,
		LteEnd:   endDate,
	}
	return
}
