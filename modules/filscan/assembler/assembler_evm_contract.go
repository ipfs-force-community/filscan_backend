package assembler

import (
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type EvmContract struct {
}

func (e EvmContract) EvmTransferToContract(transfer *bo.EvmTransfers, page, limit, index int) (contract *filscan.EvmContract) {
	contract = &filscan.EvmContract{
		Rank:            limit*(page) + index + 1,
		ActorID:         transfer.ActorID,
		ActorAddress:    transfer.ActorAddress,
		ContractAddress: transfer.ContractAddress,
		ContractName:    transfer.ContractName,
		ActorBalance:    &transfer.ActorBalance,
		TransferCount:   transfer.TransferCount,
		UserCount:       transfer.UserCount,
		GasCost:         transfer.GasCost,
	}
	return
}

//func (e EvmContract) EvmTransferStatsToContract(transfer *bo.EVMTransferStatsWithName) (contract *filscan.EvmContract) {
//	contract = &filscan.EvmContract{
//		ActorID:       transfer.ActorID,
//		ContractName:  transfer.ContractName,
//		TransferCount: transfer.AccTransferCount,
//		UserCount:     transfer.AccUserCount,
//		GasCost:       transfer.AccGasCost,
//	}
//	return
//}

func (e EvmContract) EvmTransferStatToContract(transfer *po.EvmTransferStat, page, limit, index int) (contract *filscan.EvmContract) {
	contract = &filscan.EvmContract{
		Rank:            limit*(page) + index + 1,
		ActorID:         transfer.ActorID,
		ActorAddress:    transfer.ActorAddress,
		ContractAddress: transfer.ContractAddress,
		ContractName:    transfer.ContractName,
		ActorBalance:    &transfer.ActorBalance,
		TransferCount:   transfer.AccTransferCount,
		UserCount:       transfer.AccUserCount,
		GasCost:         transfer.AccGasCost,
	}
	return
}

func (e EvmContract) EvmTransferStatsToContract(transfer *po.EvmTransferStat) (contract *filscan.EvmContract) {
	contract = &filscan.EvmContract{
		ActorID:       transfer.ActorID,
		ContractName:  transfer.ContractName,
		TransferCount: transfer.AccTransferCount,
		UserCount:     transfer.AccUserCount,
		GasCost:       transfer.AccGasCost,
	}
	return
}

func (e EvmContract) EvmSignatureToEvent(signature *londobell.Event) (event *filscan.Event) {
	var cid string
	if signature.SignedCid != "" {
		cid = signature.SignedCid
	} else {
		cid = signature.Cid
	}
	event = &filscan.Event{
		ActorID:   chain.SmartAddress(signature.ActorID).Address(),
		Epoch:     signature.Epoch,
		Cid:       cid,
		EventName: "",
		Topics:    signature.Topics,
		Data:      signature.Data,
		LogIndex:  signature.LogIndex,
		Removed:   signature.Removed,
	}
	return
}
