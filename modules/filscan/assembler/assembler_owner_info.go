package assembler

import (
	"math/big"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type OwnerInfoAssembler struct {
}

func (o OwnerInfoAssembler) ToOwnerInfoResponse(ownerBo *bo.OwnerInfo) (ownerFil *filscan.AccountOwner, err error) {
	newOwnerInfo := &filscan.AccountOwner{
		AccountIndicator: &filscan.AccountIndicator{
			AccountID:              ownerBo.Owner,
			Balance:                ownerBo.Balance,
			AvailableBalance:       ownerBo.AvailableBalance,
			InitPledge:             ownerBo.InitialPledge,
			PreDeposits:            ownerBo.PreCommitDeposits,
			LockedBalance:          ownerBo.LockedBalance,
			QualityAdjustPower:     ownerBo.QualityAdjPower,
			QualityPowerRank:       ownerBo.QualityAdjPowerRank,
			QualityPowerPercentage: ownerBo.QualityAdjPowerPercent,
			RawPower:               ownerBo.RawBytePower,
			TotalBlockCount:        ownerBo.AccBlockCount,
			TotalWinCount:          ownerBo.AccWinCount,
			TotalReward:            ownerBo.AccReward,
			//SectorSize:         ownerBo.SectorSize,
			SectorCount:          ownerBo.SectorCount,
			LiveSectorCount:      ownerBo.LiveSectorCount,
			FaultSectorCount:     ownerBo.FaultSectorCount,
			RecoverSectorCount:   ownerBo.RecoverSectorCount,
			ActiveSectorCount:    ownerBo.ActiveSectorCount,
			TerminateSectorCount: ownerBo.TerminateSectorCount,
		},
	}
	ownerFil = newOwnerInfo
	return
}

func (o OwnerInfoAssembler) ToOwnerIndicatorResponse(ownerBo *bo.ActorIndicator, powerRatio decimal.Decimal, sectorRatio decimal.Decimal) (ownerFil *filscan.MinerIndicators, err error) {
	newOwnerIndicator := &filscan.MinerIndicators{
		PowerIncrease:       ownerBo.QualityAdjPowerChange,
		PowerRatio:          powerRatio,
		SectorIncrease:      ownerBo.SealPowerChange,
		SectorRatio:         sectorRatio,
		SectorDeposits:      ownerBo.InitialPledgeChange,
		GasFee:              ownerBo.AccSealGas.Add(ownerBo.AccWdPostGas),
		BlockCountIncrease:  ownerBo.AccBlockCount,
		BlockRewardIncrease: ownerBo.AccReward,
		WinCount:            ownerBo.AccWinCount,
		RewardsPerTB:        decimal.Decimal{},
		GasFeePerTB:         decimal.Decimal{},
		Lucky:               decimal.Decimal{},
		WindowPoStGas:       ownerBo.AccWdPostGas,
	}
	if ownerBo.QualityAdjPower.GreaterThan(decimal.Zero) {
		newOwnerIndicator.RewardsPerTB = ownerBo.AccReward.Div(ownerBo.QualityAdjPower.Div(chain.PerT))
	}
	if ownerBo.SealPowerChange.GreaterThan(decimal.Zero) {
		newOwnerIndicator.GasFeePerTB = ownerBo.AccSealGas.Add(ownerBo.InitialPledgeChange).Div(ownerBo.SealPowerChange.Div(chain.PerT))
	}
	ownerFil = newOwnerIndicator
	return
}

func (o OwnerInfoAssembler) ToOwnerBalanceTrendResponse(ownerBo []*bo.ActorBalanceTrend) (ownerFil []*filscan.BalanceTrend, err error) {
	for _, ownerBalance := range ownerBo {
		balanceTrend := &filscan.BalanceTrend{
			Height:            big.NewInt(ownerBalance.Epoch),
			BlockTime:         chain.Epoch(ownerBalance.Epoch).Unix(),
			Balance:           ownerBalance.Balance,
			AvailableBalance:  ownerBalance.AvailableBalance,
			InitialPledge:     ownerBalance.InitialPledge,
			LockedFunds:       ownerBalance.LockedBalance,
			PreCommitDeposits: ownerBalance.PreCommitDeposits,
		}
		ownerFil = append(ownerFil, balanceTrend)
	}
	return
}

func (o OwnerInfoAssembler) ToOwnerPowerTrendResponse(ownerBo []*bo.ActorPowerTrend) (ownerFil []*filscan.PowerTrend, err error) {
	for _, ownerPower := range ownerBo {
		powerTrend := &filscan.PowerTrend{
			BlockTime:     chain.Epoch(ownerPower.Epoch).Unix(),
			Power:         ownerPower.Power,
			PowerIncrease: ownerPower.PowerChange,
		}
		ownerFil = append(ownerFil, powerTrend)
	}
	return
}
