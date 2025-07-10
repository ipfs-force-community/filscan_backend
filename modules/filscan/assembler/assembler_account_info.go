package assembler

import (
	"encoding/json"
	"math/big"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/message"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_tool"
)

type ActorInfo struct {
}

func (a ActorInfo) ActorStateToAccountBasic(source *londobell.ActorState) (target *filscan.AccountBasic) {
	if source.ActorType == types.ACCOUNT || source.ActorType == types.MULTISIG || source.ActorType == types.MINER {
		target = &filscan.AccountBasic{
			AccountID:      source.ActorID,
			AccountAddress: source.ActorAddr,
			AccountType:    source.ActorType,
			AccountBalance: source.Balance,
			//MessageCount:       0,
			Nonce:   source.Nonce,
			CodeCid: source.Code.String(),
			//CreateTime:         source,
			//LatestTransferTime: 0,
		}
	} else {
		target = &filscan.AccountBasic{
			AccountID:      source.ActorID,
			AccountAddress: source.DelegatedAddr,
			AccountType:    source.ActorType,
			AccountBalance: source.Balance,
			//MessageCount:       0,
			Nonce:   source.Nonce,
			CodeCid: source.Code.String(),
			//CreateTime:         source,
			//LatestTransferTime: 0,
		}
	}

	return
}

func (a ActorInfo) ActorStateToAccountSigners(source *londobell.ActorState) (target *filscan.AccountMultisig, err error) {
	mutiSigState := londobell.MutisigActorState{}
	actorState, err := json.Marshal(source.State)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(actorState, &mutiSigState)
	if err != nil {
		return nil, err
	}
	elapsedEpoch := source.Epoch - mutiSigState.StartEpoch
	lockedBalance := a.amountLocked(elapsedEpoch, mutiSigState)
	endEpoch := mutiSigState.StartEpoch + mutiSigState.UnlockDuration
	var lockedBalancePercentage decimal.Decimal
	if mutiSigState.InitialBalance.IsZero() {
		lockedBalancePercentage = decimal.Zero
	} else {
		lockedBalancePercentage = mutiSigState.InitialBalance.Sub(lockedBalance).Div(mutiSigState.InitialBalance)
	}
	availableBalance := source.Balance
	if lockedBalance.GreaterThan(decimal.Zero) {
		availableBalance = availableBalance.Sub(lockedBalance)
	}
	target = &filscan.AccountMultisig{
		AccountBasic: &filscan.AccountBasic{
			AccountID:      source.ActorID,
			AccountAddress: source.ActorAddr,
			AccountType:    source.ActorType,
			AccountBalance: source.Balance,
			//MessageCount:       0,
			Nonce:   source.Nonce,
			CodeCid: source.Code.String(),
			//CreateTime:         source,
			//LatestTransferTime: 0,
		},
		AvailableBalance:        availableBalance,
		InitialBalance:          mutiSigState.InitialBalance,
		UnlockStartTime:         chain.Epoch(mutiSigState.StartEpoch).Unix(),
		UnlockEndTime:           chain.Epoch(endEpoch).Unix(),
		LockedBalance:           lockedBalance,
		LockedBalancePercentage: lockedBalancePercentage,
		Signers:                 mutiSigState.Signers,
		ApprovalsThreshold:      mutiSigState.NumApprovalsThreshold,
	}
	return
}

func (a ActorInfo) amountLocked(elapsedEpoch int64, st londobell.MutisigActorState) decimal.Decimal {
	if elapsedEpoch >= st.UnlockDuration {
		return decimal.NewFromInt(0)
	}
	if elapsedEpoch <= 0 {
		return st.InitialBalance
	}

	unlockDuration := decimal.NewFromInt(st.UnlockDuration)
	remainingLockDuration := unlockDuration.Sub(decimal.NewFromInt(elapsedEpoch))

	numerator := st.InitialBalance.Mul(remainingLockDuration)
	denominator := unlockDuration
	quot := numerator.Div(denominator)
	rem := numerator.Mod(denominator)

	locked := quot
	if !rem.IsZero() {
		locked = locked.Add(decimal.NewFromInt(1))
	}
	return locked
}

func (a ActorInfo) MinerDetailToAccountMiner(source *londobell.MinerDetail) (target *filscan.AccountMiner) {
	controllers := _tool.RemoveRepByLoop(source.Controllers)
	target = &filscan.AccountMiner{
		AccountBasic: &filscan.AccountBasic{
			AccountID:          source.Miner,
			AccountAddress:     "",
			AccountType:        "",
			AccountBalance:     source.Balance,
			MessageCount:       0,
			Nonce:              0,
			CodeCid:            "",
			CreateTime:         nil,
			LatestTransferTime: nil,
		},
		AccountIndicator: &filscan.AccountIndicator{
			AccountID:              source.Miner,
			Balance:                source.Balance,
			AvailableBalance:       source.AvailableBalance,
			InitPledge:             source.InitialPledgeRequirement,
			PreDeposits:            source.LockedFunds,
			LockedBalance:          source.VestingFunds,
			QualityAdjustPower:     source.QualityPower,
			QualityPowerRank:       0,
			QualityPowerPercentage: decimal.Decimal{},
			RawPower:               source.Power,
			TotalBlockCount:        0,
			TotalWinCount:          0,
			TotalReward:            decimal.Decimal{},
			SectorSize:             source.SectorSize,
			SectorCount:            source.SectorCount,
			LiveSectorCount:        source.LiveSectorCount,
			FaultSectorCount:       source.FaultSectorCount,
			RecoverSectorCount:     source.RecoverSectorCount,
			ActiveSectorCount:      source.ActiveSectorCount,
			TerminateSectorCount:   source.TerminateSectorCount,
		},
		PeerID:             source.PeerID,
		OwnerAddress:       source.Owner,
		WorkerAddress:      source.Worker,
		ControllersAddress: controllers,
		BeneficiaryAddress: source.Beneficiary,
	}
	return
}

func (a ActorInfo) OwnedMinersToOwnerIndicator(ownedMiners []*filscan.AccountMiner) (ownerIndicator *filscan.AccountIndicator) {
	var accountIndicator filscan.AccountIndicator
	for _, miner := range ownedMiners {
		accountIndicator.Balance = accountIndicator.Balance.Add(miner.AccountIndicator.Balance)
		accountIndicator.AvailableBalance = accountIndicator.AvailableBalance.Add(miner.AccountIndicator.AvailableBalance)
		accountIndicator.InitPledge = accountIndicator.InitPledge.Add(miner.AccountIndicator.InitPledge)
		accountIndicator.PreDeposits = accountIndicator.PreDeposits.Add(miner.AccountIndicator.PreDeposits)
		accountIndicator.LockedBalance = accountIndicator.LockedBalance.Add(miner.AccountIndicator.LockedBalance)
		accountIndicator.QualityAdjustPower = accountIndicator.QualityAdjustPower.Add(miner.AccountIndicator.QualityAdjustPower)
		accountIndicator.RawPower = accountIndicator.RawPower.Add(miner.AccountIndicator.RawPower)
		accountIndicator.SectorCount = accountIndicator.SectorCount + miner.AccountIndicator.SectorCount
		accountIndicator.ActiveSectorCount = accountIndicator.ActiveSectorCount + miner.AccountIndicator.ActiveSectorCount
		accountIndicator.LiveSectorCount = accountIndicator.LiveSectorCount + miner.AccountIndicator.LiveSectorCount
		accountIndicator.FaultSectorCount = accountIndicator.FaultSectorCount + miner.AccountIndicator.FaultSectorCount
		accountIndicator.RecoverSectorCount = accountIndicator.RecoverSectorCount + miner.AccountIndicator.RecoverSectorCount
		accountIndicator.TerminateSectorCount = accountIndicator.TerminateSectorCount + miner.AccountIndicator.TerminateSectorCount
	}
	ownerIndicator = &accountIndicator
	return
}

func (a ActorInfo) ActorMessageToMessageBasic(source *londobell.ActorMessages) (target *filscan.MessageBasic) {
	target = &filscan.MessageBasic{
		Height:     big.NewInt(source.Epoch),
		BlockTime:  uint64(chain.Epoch(source.Epoch).Unix()),
		From:       source.From.Address(),
		To:         source.To.Address(),
		Value:      source.Value,
		ExitCode:   message.ExitCode(source.ExitCode).String(),
		MethodName: source.Method,
	}
	if source.RootCid != "" {
		target.Cid = source.RootCid
	} else if source.SignedCid != "" {
		target.Cid = source.SignedCid
	} else {
		target.Cid = source.Cid
	}

	if target.MethodName == "" {
		target.MethodName = "Other"
	}
	return
}

func (a ActorInfo) ActorBalanceToBalanceTrend(source *londobell.ActorBalance) (target *filscan.BalanceTrend) {
	target = &filscan.BalanceTrend{
		Height:    big.NewInt(source.Epoch),
		BlockTime: chain.Epoch(source.Epoch).Unix(),
		Balance:   source.Balance,
	}
	return
}

func (a ActorInfo) ToActorBalanceTrendResponse(actorBo []*bo.ActorBalanceTrend, currentActor *filscan.BalanceTrend) (actorBalanceTrends []*filscan.BalanceTrend) {
	actorBalanceTrends = append(actorBalanceTrends, currentActor)
	for _, minerBalance := range actorBo {
		balanceTrend := &filscan.BalanceTrend{
			Height:            big.NewInt(minerBalance.Epoch),
			BlockTime:         chain.Epoch(minerBalance.Epoch).Unix(),
			Balance:           minerBalance.Balance,
			AvailableBalance:  minerBalance.AvailableBalance,
			InitialPledge:     minerBalance.InitialPledge,
			LockedFunds:       minerBalance.LockedBalance,
			PreCommitDeposits: minerBalance.PreCommitDeposits,
		}
		actorBalanceTrends = append(actorBalanceTrends, balanceTrend)
	}
	return
}
