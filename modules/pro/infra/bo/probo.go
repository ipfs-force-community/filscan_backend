package probo

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type GroupMiners struct {
	GroupID   int64
	GroupName string
	IsDefault bool
	MinersID  []*MinerInfo
}

type GroupMinerInfo struct {
	GroupID  int64
	MinerID  chain.SmartAddress
	MinerTag string
}

type MinerInfo struct {
	MinerID  chain.SmartAddress
	MinerTag string
}

type UserMiner struct {
	UserID    int64
	GroupID   int64
	GroupName string
	IsDefault bool
	MinerID   chain.SmartAddress
	MinerTag  string
}

func UserMinersToGroupMiners(input []*UserMiner) (output []*GroupMiners) {
	groupMiners := make(map[int64][]*MinerInfo)
	for _, miner := range input {
		groupMiners[miner.GroupID] = append(groupMiners[miner.GroupID], &MinerInfo{
			MinerID:  miner.MinerID,
			MinerTag: miner.MinerTag,
		})
	}
	for _, v := range input {
		output = append(output, &GroupMiners{
			GroupID:   v.GroupID,
			GroupName: v.GroupName,
			IsDefault: v.IsDefault,
			MinersID:  groupMiners[v.GroupID],
		})
	}
	return
}

func ConvertGroupMiners(input []*GroupMiners) (minerIDList []chain.SmartAddress, minerInfo map[chain.SmartAddress]UserMiner) {
	minerInfo = make(map[chain.SmartAddress]UserMiner)
	minerIDMap := make(map[chain.SmartAddress]chain.SmartAddress)
	for _, miners := range input {
		for _, miner := range miners.MinersID {
			minerIDMap[miner.MinerID] = miner.MinerID
			minerInfo[miner.MinerID] = UserMiner{
				GroupID:   miners.GroupID,
				GroupName: miners.GroupName,
				IsDefault: miners.IsDefault,
				MinerID:   miner.MinerID,
				MinerTag:  miner.MinerTag,
			}
		}
	}
	for minerID := range minerIDMap {
		minerIDList = append(minerIDList, minerID)
	}
	return
}
