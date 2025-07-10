package convertor

//type OwnerInfoConvertor struct {
//}
//
//func (OwnerInfoConvertor) ToOwnerInfoPo(source *owner.Info) (target *po.OwnerInfo, err error) {
//	
//	target = &po.OwnerInfo{
//		Epoch:                    source.Epoch.Int64(),
//		Owner:                    source.Owner.Address(),
//		Miners:                   nil,
//		RawBytePower:             source.RawBytePower.Decimal(),
//		QualityAdjPower:          source.QualityAdjPower.Decimal(),
//		Balance:                  source.Balance.Decimal(),
//		AvailableBalance:         source.AvailableBalance.Decimal(),
//		VestingFunds:             source.VestingFunds.Decimal(),
//		FeeDebt:                  source.FeeDebt.Decimal(),
//		SectorSize:               source.SectorSize,
//		SectorCount:              source.SectorCount,
//		FaultSectorCount:         source.FaultSectorCount,
//		ActiveSectorCount:        source.ActiveSectorCount,
//		LiveSectorCount:          source.LiveSectorCount,
//		RecoverSectorCount:       source.RecoverSectorCount,
//		TerminateSectorCount:     source.TerminateSectorCount,
//		PreCommitSectorCount:     source.PreCommitSectorCount,
//		InitialPledge:            source.InitialPledge.Decimal(),
//		PreCommitDeposits:        source.PreCommitDeposits.Decimal(),
//		RawBytePower24hChange:    source.RawBytePower24hChange.Decimal(),
//		QualityAdjPower24hChange: source.QualityAdjPower24hChange.Decimal(),
//		AccReward24h:             source.AccReward24h.Decimal(),
//		Prev24hEpochRef:          source.Prev24hEpochRef.Int64(),
//		RewardPowerRatio24h:      source.RewardPowerRatio24h.Decimal(),
//		AccBlockCount24h:         source.AccBlockCount24h,
//	}
//	for _, v := range source.Miners {
//		target.Miners = append(target.Miners, v.Address())
//	}
//	return
//}
//
//func (OwnerInfoConvertor) ToOwnerInfoEntity(source *po.OwnerInfo) (target *owner.Info, err error) {
//	target = &owner.Info{
//		Epoch:                    chain.Epoch(source.Epoch),
//		Owner:                    chain.SmartAddress(source.Owner),
//		Miners:                   nil,
//		RawBytePower:             chain.Power(source.RawBytePower),
//		QualityAdjPower:          chain.Power(source.QualityAdjPower),
//		Balance:                  chain.AttoFil(source.Balance),
//		AvailableBalance:         chain.AttoFil(source.AvailableBalance),
//		VestingFunds:             chain.AttoFil(source.VestingFunds),
//		FeeDebt:                  chain.AttoFil(source.FeeDebt),
//		SectorSize:               source.SectorSize,
//		SectorCount:              source.SectorCount,
//		FaultSectorCount:         source.FaultSectorCount,
//		ActiveSectorCount:        source.ActiveSectorCount,
//		LiveSectorCount:          source.LiveSectorCount,
//		RecoverSectorCount:       source.RecoverSectorCount,
//		TerminateSectorCount:     source.TerminateSectorCount,
//		PreCommitSectorCount:     source.PreCommitSectorCount,
//		InitialPledge:            chain.AttoFil(source.InitialPledge),
//		PreCommitDeposits:        chain.AttoFil(source.PreCommitDeposits),
//		RawBytePower24hChange:    chain.Power(source.RawBytePower24hChange),
//		QualityAdjPower24hChange: chain.Power(source.QualityAdjPower24hChange),
//		AccReward24h:             chain.AttoFil(source.AccReward24h),
//		AccBlockCount24h:         source.AccBlockCount24h,
//		Prev24hEpochRef:          chain.Epoch(source.Prev24hEpochRef),
//		RewardPowerRatio24h:      chain.Ratio(source.RewardPowerRatio24h),
//	}
//	for _, v := range source.Miners {
//		target.Miners = append(target.Miners, chain.SmartAddress(v))
//	}
//	return
//}
