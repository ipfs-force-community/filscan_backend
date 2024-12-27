package assembler

import (
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"math/big"
)

type MinerInfoAssembler struct {
}

func (m MinerInfoAssembler) ToMinerInfoResponse(minerBo *bo.MinerInfo) (minerFil *filscan.AccountMiner, err error) {
	newMinerInfo := &filscan.AccountMiner{
		AccountIndicator: &filscan.AccountIndicator{
			AccountID:              minerBo.Miner,
			Balance:                minerBo.Balance,
			AvailableBalance:       minerBo.AvailableBalance,
			InitPledge:             minerBo.InitialPledge,
			PreDeposits:            minerBo.PreCommitDeposits,
			LockedBalance:          minerBo.LockedBalance,
			QualityAdjustPower:     minerBo.QualityAdjPower,
			QualityPowerRank:       minerBo.QualityAdjPowerRank,
			QualityPowerPercentage: minerBo.QualityAdjPowerPercent,
			RawPower:               minerBo.RawBytePower,
			TotalBlockCount:        minerBo.AccBlockCount,
			TotalWinCount:          minerBo.AccWinCount,
			TotalReward:            minerBo.AccReward,
			SectorSize:             minerBo.SectorSize,
			SectorCount:            minerBo.SectorCount,
			LiveSectorCount:        minerBo.LiveSectorCount,
			FaultSectorCount:       minerBo.FaultSectorCount,
			RecoverSectorCount:     minerBo.RecoverSectorCount,
			ActiveSectorCount:      minerBo.ActiveSectorCount,
		},
		PeerID:             "",
		OwnerAddress:       minerBo.Owner,
		WorkerAddress:      minerBo.Worker,
		ControllersAddress: minerBo.Controllers,
		BeneficiaryAddress: "",
	}
	minerFil = newMinerInfo
	return
}

func (m MinerInfoAssembler) ToMinerIndicatorResponse(endMinerBo *bo.MinerInfo, startMinerBo *bo.MinerInfo, day decimal.Decimal) (minerFil *filscan.MinerIndicators, err error) {
	newMinerIndicator := &filscan.MinerIndicators{
		PowerIncrease:       endMinerBo.QualityAdjPower.Sub(startMinerBo.QualityAdjPower),
		PowerRatio:          endMinerBo.QualityAdjPower.Sub(startMinerBo.QualityAdjPower).Div(day),
		SectorIncrease:      decimal.NewFromInt((endMinerBo.SectorSize * endMinerBo.SectorCount) - (startMinerBo.SectorSize * startMinerBo.SectorCount)),
		SectorRatio:         decimal.NewFromInt((endMinerBo.SectorSize * endMinerBo.SectorCount) - (startMinerBo.SectorSize * startMinerBo.SectorCount)).Div(day),
		SectorDeposits:      endMinerBo.PreCommitDeposits.Sub(startMinerBo.PreCommitDeposits),
		GasFee:              decimal.Decimal{},
		BlockCountIncrease:  0,
		BlockRewardIncrease: decimal.Decimal{},
		WinCount:            0,
		RewardsPerTB:        decimal.Decimal{},
		GasFeePerTB:         decimal.Decimal{},
		Lucky:               decimal.Decimal{},
	}
	
	minerFil = newMinerIndicator
	return
}

func (m MinerInfoAssembler) ToMinerBalanceTrendResponse(minerBo []*bo.ActorBalanceTrend) (minerFil []*filscan.BalanceTrend, err error) {
	for _, minerBalance := range minerBo {
		balanceTrend := &filscan.BalanceTrend{
			Height:            big.NewInt(minerBalance.Epoch),
			BlockTime:         chain.Epoch(minerBalance.Epoch).Unix(),
			Balance:           minerBalance.Balance,
			AvailableBalance:  minerBalance.AvailableBalance,
			InitialPledge:     minerBalance.InitialPledge,
			LockedFunds:       minerBalance.LockedBalance,
			PreCommitDeposits: minerBalance.PreCommitDeposits,
			Epoch:             minerBalance.Epoch,
		}
		minerFil = append(minerFil, balanceTrend)
	}
	return
}

func (m MinerInfoAssembler) ToMinerPowerTrendResponse(minerBo []*bo.ActorPowerTrend) (minerFil []*filscan.PowerTrend, err error) {
	for _, minerPower := range minerBo {
		powerTrend := &filscan.PowerTrend{
			BlockTime:     chain.Epoch(minerPower.Epoch).Unix(),
			Power:         minerPower.Power,
			PowerIncrease: minerPower.PowerChange,
			Epoch:         minerPower.Epoch,
		}
		minerFil = append(minerFil, powerTrend)
	}
	return
}
