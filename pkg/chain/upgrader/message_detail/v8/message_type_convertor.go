package v8

import (
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	initial "github.com/filecoin-project/go-state-types/builtin/v8/init"
	"github.com/filecoin-project/go-state-types/builtin/v8/market"
	"github.com/filecoin-project/go-state-types/builtin/v8/miner"
	"github.com/filecoin-project/go-state-types/builtin/v8/multisig"
	"github.com/filecoin-project/go-state-types/builtin/v8/power"
	"github.com/filecoin-project/go-state-types/builtin/v8/verifreg"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/message"
)

type ConvertMessageType struct {
}

func (c ConvertMessageType) EmptyValue(input *abi.EmptyValue) (result interface{}, err error) {
	result = input
	return
}

func (c ConvertMessageType) CborBytes(input *abi.CborBytes) (result interface{}, err error) {
	result = message.ByteToHex(*input)
	return
}

func (c ConvertMessageType) TokenAmount(input *abi.TokenAmount) (result interface{}, err error) {
	result = input
	return
}

func (c ConvertMessageType) Address(input *address.Address) (result interface{}, err error) {
	result = input
	return
}

func (c ConvertMessageType) AddVerifiedClientParams(input *verifreg.AddVerifiedClientParams) (result interface{}, err error) {
	result = &AddVerifiedClientParams{
		Address:   input.Address.String(),
		Allowance: input.Allowance.String(),
	}
	return
}

func (c ConvertMessageType) ApproveReturn(input *multisig.ApproveReturn) (result interface{}, err error) {
	result = &ApproveReturn{
		Applied: input.Applied,
		Code:    input.Code.String(),
		Ret:     message.ByteToHex(input.Ret),
	}
	return
}

func (c ConvertMessageType) ExecReturn(input *initial.ExecReturn) (result interface{}, err error) {
	result = &ExecReturn{
		IDAddress:     input.IDAddress.String(),
		RobustAddress: input.RobustAddress.String(),
	}
	return
}

func (c ConvertMessageType) TxnIDParams(input *multisig.TxnIDParams) (result interface{}, err error) {
	result = &TxnIDParams{
		ID:           int64(input.ID),
		ProposalHash: message.ByteToHex(input.ProposalHash),
	}
	return
}

func (c ConvertMessageType) ChangeMultiaddrsParams(input *miner.ChangeMultiaddrsParams) (result interface{}, err error) {
	var newMultiaddrs []string
	for _, multiaddrs := range input.NewMultiaddrs {
		var addr multiaddr.Multiaddr
		addr, err = multiaddr.NewMultiaddrBytes(multiaddrs)
		if err != nil {
			return
		}
		newMultiaddrs = append(newMultiaddrs, addr.String())
	}
	result = &ChangeMultiaddrsParams{
		NewMultiaddrs: newMultiaddrs,
	}
	return
}

func (c ConvertMessageType) ChangePeerIDParams(input *miner.ChangePeerIDParams) (result interface{}, err error) {
	newID, err := peer.IDFromBytes(input.NewID)
	if err != nil {
		return
	}
	result = &ChangePeerIDParams{
		NewID: newID.String(),
	}
	return
}
func (c ConvertMessageType) ChangeWorkerAddressParams(input *miner.ChangeWorkerAddressParams) (result interface{}, err error) {
	var newControllerAddrs []string
	for _, addr := range input.NewControlAddrs {
		newControllerAddrs = append(newControllerAddrs, addr.String())
	}
	result = &ChangeWorkerAddressParams{
		NewWorker:       input.NewWorker.String(),
		NewControlAddrs: newControllerAddrs,
	}
	return
}

func (c ConvertMessageType) CreateMinerParams(input *power.CreateMinerParams) (result interface{}, err error) {
	var newMultiaddrs []string
	for _, multiaddrs := range input.Multiaddrs {
		var addr multiaddr.Multiaddr
		addr, err = multiaddr.NewMultiaddrBytes(multiaddrs)
		if err != nil {
			return
		}
		newMultiaddrs = append(newMultiaddrs, addr.String())
	}
	result = &CreateMinerParams{
		Owner:               input.Owner.String(),
		Worker:              input.Worker.String(),
		WindowPoStProofType: int64(input.WindowPoStProofType),
		Peer:                message.ByteToHex(input.Peer),
		Multiaddrs:          newMultiaddrs,
	}
	return
}

func (c ConvertMessageType) CreateMinerReturn(input *power.CreateMinerReturn) (result interface{}, err error) {
	result = &CreateMinerReturn{
		IDAddress:     input.IDAddress.String(),
		RobustAddress: input.RobustAddress.String(),
	}
	return
}

func (c ConvertMessageType) CompactPartitionsParams(input *miner.CompactPartitionsParams) (result interface{}, err error) {
	var bitField string
	bitField, err = message.DecodeBitField(input.Partitions)
	if err != nil {
		return
	}
	result = &CompactPartitionsParams{
		Deadline:   int64(input.Deadline),
		Partitions: bitField,
	}
	return
}

func (c ConvertMessageType) CompactSectorNumbersParams(input *miner.CompactSectorNumbersParams) (result interface{}, err error) {
	var bitField string
	bitField, err = message.DecodeBitField(input.MaskSectorNumbers)
	if err != nil {
		return
	}
	result = &CompactSectorNumbersParams{
		MaskSectorNumbers: bitField,
	}
	return
}

func (c ConvertMessageType) DeclareFaultsRecoveredParams(input *miner.DeclareFaultsRecoveredParams) (result interface{}, err error) {
	var newRecoveries []RecoveryDeclaration
	for _, recovery := range input.Recoveries {
		var bitField string
		bitField, err = message.DecodeBitField(recovery.Sectors)
		if err != nil {
			return
		}
		newRecoveries = append(newRecoveries, RecoveryDeclaration{
			Deadline:  int64(recovery.Deadline),
			Partition: int64(recovery.Partition),
			Sectors:   bitField,
		})
	}
	result = &DeclareFaultsRecoveredParams{
		Recoveries: newRecoveries,
	}
	return
}

func (c ConvertMessageType) DisputeWindowedPoStParams(input *miner.DisputeWindowedPoStParams) (result interface{}, err error) {
	result = &DisputeWindowedPoStParams{
		Deadline:  int64(input.Deadline),
		PoStIndex: int64(input.PoStIndex),
	}
	return
}

func (c ConvertMessageType) ExecParams(input *initial.ExecParams) (result interface{}, err error) {
	result = &ExecParams{
		CodeCID:           input.CodeCID.String(),
		ConstructorParams: message.ByteToHex(input.ConstructorParams),
	}
	return
}

func (c ConvertMessageType) ExtendSectorExpirationParams(input *miner.ExtendSectorExpirationParams) (result interface{}, err error) {
	var newExpirationExtension []ExpirationExtension
	for _, extension := range input.Extensions {
		var bitField string
		bitField, err = message.DecodeBitField(extension.Sectors)
		if err != nil {
			return
		}
		newExpirationExtension = append(newExpirationExtension, ExpirationExtension{
			Deadline:      int64(extension.Deadline),
			Partition:     int64(extension.Partition),
			Sectors:       bitField,
			NewExpiration: int64(extension.NewExpiration),
		})
	}
	result = &ExtendSectorExpirationParams{
		Extensions: newExpirationExtension,
	}
	return
}

func (c ConvertMessageType) PreCommitSectorParams(input *miner.PreCommitSectorParams) (result interface{}, err error) {
	var dealIDs []int64
	for _, deal := range input.DealIDs {
		dealIDs = append(dealIDs, int64(deal))
	}
	result = &PreCommitSectorParams{
		SealProof:              int64(input.SealProof),
		SectorNumber:           int64(input.SectorNumber),
		SealedCID:              input.SealedCID.String(),
		SealRandEpoch:          int64(input.SealRandEpoch),
		DealIDs:                dealIDs,
		Expiration:             int64(input.Expiration),
		ReplaceCapacity:        input.ReplaceCapacity,
		ReplaceSectorDeadline:  int64(input.ReplaceSectorDeadline),
		ReplaceSectorPartition: int64(input.ReplaceSectorPartition),
		ReplaceSectorNumber:    int64(input.SectorNumber),
	}
	return
}

func (c ConvertMessageType) PreCommitSectorBatchParams(input *miner.PreCommitSectorBatchParams) (result interface{}, err error) {
	var newSectors []PreCommitSectorParams
	for _, sector := range input.Sectors {
		var dealIDs []int64
		for _, deal := range sector.DealIDs {
			dealIDs = append(dealIDs, int64(deal))
		}
		newSectors = append(newSectors, PreCommitSectorParams{
			SealProof:              int64(sector.SealProof),
			SectorNumber:           int64(sector.SectorNumber),
			SealedCID:              sector.SealedCID.String(),
			SealRandEpoch:          int64(sector.SealRandEpoch),
			DealIDs:                dealIDs,
			Expiration:             int64(sector.Expiration),
			ReplaceCapacity:        sector.ReplaceCapacity,
			ReplaceSectorDeadline:  int64(sector.ReplaceSectorDeadline),
			ReplaceSectorPartition: int64(sector.ReplaceSectorPartition),
			ReplaceSectorNumber:    int64(sector.SectorNumber),
		})
	}
	result = &PreCommitSectorBatchParams{
		Sectors: newSectors,
	}
	return
}

func (c ConvertMessageType) ProposeParams(input *multisig.ProposeParams) (result interface{}, err error) {
	result = &ProposeParams{
		To:     input.To.String(),
		Value:  input.Value.String(),
		Method: input.Method.String(),
		Params: message.ByteToHex(input.Params),
	}
	return
}

func (c ConvertMessageType) ProposeReturn(input *multisig.ProposeReturn) (result interface{}, err error) {
	result = &ProposeReturn{
		TxnID:   int64(input.TxnID),
		Applied: input.Applied,
		Code:    input.Code.String(),
		Ret:     message.ByteToHex(input.Ret),
	}
	return
}

func (c ConvertMessageType) ProveCommitAggregateParams(input *miner.ProveCommitAggregateParams) (result interface{}, err error) {
	var bitField string
	bitField, err = message.DecodeBitField(input.SectorNumbers)
	if err != nil {
		return
	}
	result = &ProveCommitAggregateParams{
		SectorNumbers:  bitField,
		AggregateProof: message.ByteToHex(input.AggregateProof),
	}
	return
}

func (c ConvertMessageType) ProveCommitSectorParams(input *miner.ProveCommitSectorParams) (result interface{}, err error) {
	result = &ProveCommitSectorParams{
		SectorNumber: input.SectorNumber.String(),
		Proof:        message.ByteToHex(input.Proof),
	}
	return
}

func (c ConvertMessageType) ProveReplicaUpdatesParams(input *miner.ProveReplicaUpdatesParams) (result interface{}, err error) {
	var newUpdate []ReplicaUpdate
	for _, update := range input.Updates {
		var deals []int64
		for _, deal := range update.Deals {
			deals = append(deals, int64(deal))
		}
		newUpdate = append(newUpdate, ReplicaUpdate{
			SectorID:           int64(update.SectorID),
			Deadline:           int64(update.Deadline),
			Partition:          int64(update.Partition),
			NewSealedSectorCID: update.NewSealedSectorCID.String(),
			Deals:              deals,
			UpdateProofType:    int64(update.UpdateProofType),
			ReplicaProof:       message.ByteToHex(update.ReplicaProof),
		})
	}
	result = &ProveReplicaUpdatesParams{
		Updates: newUpdate,
	}
	return
}

func (c ConvertMessageType) PublishStorageDealsParams(input *market.PublishStorageDealsParams) (result interface{}, err error) {
	var newDeals []ClientDealProposal
	for _, deal := range input.Deals {
		newDeals = append(newDeals, ClientDealProposal{
			Proposal: DealProposal{
				PieceCID:             deal.Proposal.PieceCID.String(),
				PieceSize:            int64(deal.Proposal.PieceSize),
				VerifiedDeal:         deal.Proposal.VerifiedDeal,
				Client:               deal.Proposal.Client.String(),
				Provider:             deal.Proposal.Provider.String(),
				Label:                deal.Proposal.Label,
				StartEpoch:           deal.Proposal.StartEpoch.String(),
				EndEpoch:             deal.Proposal.EndEpoch.String(),
				StoragePricePerEpoch: deal.Proposal.StoragePricePerEpoch.String(),
				ProviderCollateral:   deal.Proposal.ProviderCollateral.String(),
				ClientCollateral:     deal.Proposal.ClientCollateral.String(),
			},
			ClientSignature: Signature{
				Type: byte(deal.ClientSignature.Type),
				Data: message.ByteToHex(deal.ClientSignature.Data),
			},
		})
	}
	result = &PublishStorageDealsParams{
		Deals: newDeals,
	}
	return
}

func (c ConvertMessageType) PublishStorageDealsReturn(input *market.PublishStorageDealsReturn) (result interface{}, err error) {
	var newIds []int64
	for _, id := range input.IDs {
		newIds = append(newIds, int64(id))
	}
	var bitField string
	bitField, err = message.DecodeBitField(input.ValidDeals)
	if err != nil {
		return
	}
	result = &PublishStorageDealsReturn{
		IDs:        newIds,
		ValidDeals: bitField,
	}
	return
}

func (c ConvertMessageType) ReportConsensusFaultParams(input *miner.ReportConsensusFaultParams) (result interface{}, err error) {
	result = &ReportConsensusFaultParams{
		BlockHeader1:     message.ByteToHex(input.BlockHeader1),
		BlockHeader2:     message.ByteToHex(input.BlockHeader2),
		BlockHeaderExtra: message.ByteToHex(input.BlockHeaderExtra),
	}
	return
}

func (c ConvertMessageType) SubmitWindowedPoStParams(input *miner.SubmitWindowedPoStParams) (result interface{}, err error) {
	var newPartitions []PoStPartition
	for _, partition := range input.Partitions {
		var bitField string
		bitField, err = message.DecodeBitField(partition.Skipped)
		if err != nil {
			return
		}
		newPartitions = append(newPartitions, PoStPartition{
			Index:   int64(partition.Index),
			Skipped: bitField,
		})
	}
	var newPoStProof []PoStProof
	for _, proof := range input.Proofs {
		newPoStProof = append(newPoStProof, PoStProof{
			PoStProof:  int64(proof.PoStProof),
			ProofBytes: message.ByteToHex(proof.ProofBytes),
		})
	}
	result = &SubmitWindowedPoStParams{
		Deadline:         int64(input.Deadline),
		Partitions:       newPartitions,
		Proofs:           newPoStProof,
		ChainCommitEpoch: input.ChainCommitEpoch.String(),
		ChainCommitRand:  message.ByteToHex(input.ChainCommitRand),
	}
	return
}

func (c ConvertMessageType) TerminateSectorsParams(input *miner.TerminateSectorsParams) (result interface{}, err error) {
	var newTerminations []TerminationDeclaration
	for _, termination := range input.Terminations {
		var bitField string
		bitField, err = message.DecodeBitField(termination.Sectors)
		if err != nil {
			return
		}
		newTerminations = append(newTerminations, TerminationDeclaration{
			Deadline:  int64(termination.Deadline),
			Partition: int64(termination.Partition),
			Sectors:   bitField,
		})
	}
	result = &TerminateSectorsParams{
		Terminations: newTerminations,
	}
	return
}

func (c ConvertMessageType) TerminateSectorsReturn(input *miner.TerminateSectorsReturn) (result interface{}, err error) {
	result = &TerminateSectorsReturn{
		Done: input.Done,
	}
	return
}

func (c ConvertMessageType) WithdrawBalanceParamsMarket(input *market.WithdrawBalanceParams) (result interface{}, err error) {
	result = &WithdrawBalanceParamsMarket{
		ProviderOrClientAddress: input.ProviderOrClientAddress.String(),
		Amount:                  input.Amount.String(),
	}
	return
}

func (c ConvertMessageType) WithdrawBalanceParamsMiner(input *miner.WithdrawBalanceParams) (result interface{}, err error) {
	result = &WithdrawBalanceParamsMiner{
		AmountRequested: input.AmountRequested.String(),
	}
	return
}
