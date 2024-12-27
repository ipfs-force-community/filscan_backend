package convertor

//type MinerInfoConvertor struct {
//}
//
//func (MinerInfoConvertor) ToMinerInfoPo(source *miner.Info) (target *po.MinerInfo, err error) {
//
//	target = &po.MinerInfo{
//		Epoch:                    source.Epoch.Int64(),
//		Miner:                    source.Miner.Address(),
//		Owner:                    source.Owner.Address(),
//		Worker:                   source.Owner.Address(),
//		Controllers:              types.StringArray{},
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
//		QualityAdjPowerPercent:   source.QualityAdjPowerPercent.Decimal(),
//		AccReward24hPercent:      source.AccReward24hPercent.Decimal(),
//		AccBlockCount24hPercent:  source.AccBlockCount24hPercent.Decimal(),
//		AccBlockCount24h:         source.AccBlockCount24,
//	}
//	for _, v := range source.Controllers {
//		target.Controllers = append(target.Controllers, v.Address())
//	}
//	return
//}
//
//func (MinerInfoConvertor) ToMinerInfoEntity(source *po.MinerInfo) (target *miner.Info, err error) {
//	target = &miner.Info{
//		Epoch:                    chain.Epoch(source.Epoch),
//		Miner:                    chain.SmartAddress(source.Miner),
//		Owner:                    chain.SmartAddress(source.Owner),
//		Worker:                   chain.SmartAddress(source.Worker),
//		Controllers:              nil,
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
//		QualityAdjPowerPercent:   chain.Ratio(source.QualityAdjPowerPercent),
//		AccReward24h:             chain.AttoFil(source.AccReward24h),
//		AccBlockCount24:          source.AccBlockCount24h,
//		Prev24hEpochRef:          chain.Epoch(source.Prev24hEpochRef),
//		AccReward24hPercent:      chain.Ratio(source.AccReward24hPercent),
//		AccBlockCount24hPercent:  chain.Ratio(source.AccBlockCount24hPercent),
//	}
//	for _, v := range source.Controllers {
//		target.Controllers = append(target.Controllers, chain.SmartAddress(v))
//	}
//	return
//}
