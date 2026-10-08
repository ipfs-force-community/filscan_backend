package v19

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v19/datacap"
	initial "github.com/filecoin-project/go-state-types/builtin/v19/init"
	"github.com/filecoin-project/go-state-types/builtin/v19/market"
	"github.com/filecoin-project/go-state-types/builtin/v19/miner"
	"github.com/filecoin-project/go-state-types/builtin/v19/multisig"
	"github.com/filecoin-project/go-state-types/builtin/v19/power"
	"github.com/filecoin-project/go-state-types/builtin/v19/reward"
	"github.com/filecoin-project/go-state-types/builtin/v19/verifreg"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/message"
)

var newParams = map[string]interface{}{
	"AddBalance":                 address.Address{},
	"AddVerifiedClient":          verifreg.AddVerifiedClientParams{},
	"AllowanceExported":          datacap.GetAllowanceParams{},
	"Approve":                    multisig.TxnIDParams{},
	"Cancel":                     multisig.TxnIDParams{},
	"CancelExported":             multisig.TxnIDParams{},
	"GetBeneficiary":             abi.EmptyValue{},
	"GetBeneficiaryExported":     abi.EmptyValue{},
	"MaxTerminationFee":          miner.MaxTerminationFeeParams{},
	"ChangeBeneficiary":          miner.ChangeBeneficiaryParams{},
	"ChangeMultiaddrs":           miner.ChangeMultiaddrsParams{},
	"ChangeOwnerAddress":         address.Address{},
	"ChangePeerID":               miner.ChangePeerIDParams{},
	"ChangeWorkerAddress":        miner.ChangeWorkerAddressParams{},
	"CompactPartitions":          miner.CompactPartitionsParams{},
	"CompactSectorNumbers":       miner.CompactSectorNumbersParams{},
	"ConfirmChangeWorkerAddress": abi.EmptyValue{},
	"Constructor":                address.Address{},
	"CreateExternal":             abi.CborBytes{},
	"CreateMiner":                power.CreateMinerParams{},
	"DeclareFaultsRecovered":     miner.DeclareFaultsRecoveredParams{},
	"DisputeWindowedPoSt":        miner.DisputeWindowedPoStParams{},
	"Exec":                       initial.ExecParams{},
	"ExtendClaimTerms":           verifreg.ExtendClaimTermsParams{},
	"ExtendSectorExpiration":     miner.ExtendSectorExpirationParams{},
	"ExtendSectorExpiration2":    miner.ExtendSectorExpiration2Params{},
	"IncreaseAllowanceExported":  datacap.IncreaseAllowanceParams{},
	"InvokeContract":             abi.CborBytes{},
	"PreCommitSector":            miner.PreCommitSectorParams{},
	"PreCommitSectorBatch":       miner.PreCommitSectorBatchParams{},
	"PreCommitSectorBatch2":      miner.PreCommitSectorBatchParams2{},
	"Propose":                    multisig.ProposeParams{},
	"ProveCommitAggregate":       miner.ProveCommitAggregateParams{},
	"ProveCommitSector":          miner.ProveCommitSectorParams{},
	"ProveReplicaUpdates":        miner.ProveReplicaUpdatesParams{},
	"PubkeyAddress":              abi.EmptyValue{},
	"PublishStorageDeals":        market.PublishStorageDealsParams{},
	"RemoveExpiredAllocations":   verifreg.RemoveExpiredAllocationsParams{},
	"RepayDebt":                  abi.EmptyValue{},
	"ReportConsensusFault":       miner.ReportConsensusFaultParams{},
	"SubmitWindowedPoSt":         miner.SubmitWindowedPoStParams{},
	"TerminateSectors":           miner.TerminateSectorsParams{},
	"TransferFromExported":       datacap.TransferFromParams{},
	"UpgradeSectorQuality":       miner.UpgradeSectorQualityParams{},
	"WithdrawBalance(miner)":     miner.WithdrawBalanceParams{},
	"WithdrawBalance(market)":    market.WithdrawBalanceParams{},
	// NV29(Solstice / FIP-0118) 奖励流方法（f02 的 9 个 FRC 方法）。
	// 名字与类型以 go-state-types v0.19.0 builtin/v19/reward 为准（methods.go / reward_types.go）。
	"SetWeightRecordsExported":  reward.SetWeightRecordsParams{},
	"StepWeightRecordsExported": reward.StepWeightRecordsParams{},
	"RegisterStreamExported":    reward.RegisterStreamParams{},
	"RemoveStreamExported":      reward.RemoveStreamParams{},
	"SetDistributionExported":   reward.SetDistributionParams{},
	"SetSharesExported":         reward.SetSharesParams{},
	"ReplaceAddressExported":    reward.ReplaceAddressParams{},
	"CancelPendingExported":     reward.CancelPendingParams{},
	"ClaimExported":             reward.ClaimParams{},
}

func DecodeMessageParams(input interface{}, methodName string) (result interface{}, err error) {
	method := newParams[methodName]
	if method == nil {
		return
	}

	paramsByte, err := message.DecodeMessage(input)
	if err != nil {
		return
	}

	params := reflect.New(reflect.TypeOf(method)).Interface()

	switch v := params.(type) {
	case *abi.EmptyValue:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.EmptyValue(v)
			if err != nil {
				return
			}
		}
	case *address.Address:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.Address(v)
			if err != nil {
				return
			}
		}
	case *verifreg.AddVerifiedClientParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.AddVerifiedClientParams(v)
			if err != nil {
				return
			}
		}
	case *datacap.GetAllowanceParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.GetAllowanceParams(v)
			if err != nil {
				return
			}
		}
	case *multisig.TxnIDParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.TxnIDParams(v)
			if err != nil {
				return
			}
		}
	case *miner.MaxTerminationFeeParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.MaxTerminationFeeParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ChangeBeneficiaryParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ChangeBeneficiaryParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ChangeMultiaddrsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ChangeMultiaddrsParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ChangePeerIDParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ChangePeerIDParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ChangeWorkerAddressParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ChangeWorkerAddressParams(v)
			if err != nil {
				return
			}
		}
	case *miner.CompactPartitionsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.CompactPartitionsParams(v)
			if err != nil {
				return
			}
		}
	case *miner.CompactSectorNumbersParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.CompactSectorNumbersParams(v)
			if err != nil {
				return
			}
		}
	case *abi.CborBytes:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.CborBytes(v)
			if err != nil {
				return
			}
		}
	case *power.CreateMinerParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.CreateMinerParams(v)
			if err != nil {
				return
			}
		}
	case *miner.DeclareFaultsRecoveredParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.DeclareFaultsRecoveredParams(v)
			if err != nil {
				return
			}
		}
	case *miner.DisputeWindowedPoStParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.DisputeWindowedPoStParams(v)
			if err != nil {
				return
			}
		}
	case *initial.ExecParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ExecParams(v)
			if err != nil {
				return
			}
		}
	case *verifreg.ExtendClaimTermsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ExtendClaimTermsParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ExtendSectorExpirationParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ExtendSectorExpirationParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ExtendSectorExpiration2Params:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ExtendSectorExpiration2Params(v)
			if err != nil {
				return
			}
		}
	case *datacap.IncreaseAllowanceParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.IncreaseAllowanceParams(v)
			if err != nil {
				return
			}
		}
	case *miner.PreCommitSectorParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.PreCommitSectorParams(v)
			if err != nil {
				return
			}
		}
	case *miner.PreCommitSectorBatchParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.PreCommitSectorBatchParams(v)
			if err != nil {
				return
			}
		}
	case *miner.PreCommitSectorBatchParams2:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.PreCommitSectorBatchParams2(v)
			if err != nil {
				return
			}
		}
	case *multisig.ProposeParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ProposeParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ProveCommitAggregateParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ProveCommitAggregateParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ProveCommitSectorParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ProveCommitSectorParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ProveReplicaUpdatesParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ProveReplicaUpdatesParams(v)
			if err != nil {
				return
			}
		}
	case *market.PublishStorageDealsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.PublishStorageDealsParams(v)
			if err != nil {
				return
			}
		}
	case *verifreg.RemoveExpiredAllocationsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.RemoveExpiredAllocationsParams(v)
			if err != nil {
				return
			}
		}
	case *miner.ReportConsensusFaultParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ReportConsensusFaultParams(v)
			if err != nil {
				return
			}
		}
	case *miner.SubmitWindowedPoStParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.SubmitWindowedPoStParams(v)
			if err != nil {
				return
			}
		}
	case *miner.TerminateSectorsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.TerminateSectorsParams(v)
			if err != nil {
				return
			}
		}
	case *miner.UpgradeSectorQualityParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.UpgradeSectorQualityParams(v)
			if err != nil {
				return
			}
		}
	case *datacap.TransferFromParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.TransferFromParams(v)
			if err != nil {
				return
			}
		}
	case *market.WithdrawBalanceParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.WithdrawBalanceParamsMarket(v)
			if err != nil {
				return
			}
		}
	case *miner.WithdrawBalanceParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.WithdrawBalanceParamsMiner(v)
			if err != nil {
				return
			}
		}
	case *reward.SetWeightRecordsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.SetWeightRecordsParams(v)
			if err != nil {
				return
			}
		}
	case *reward.StepWeightRecordsParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.StepWeightRecordsParams(v)
			if err != nil {
				return
			}
		}
	case *reward.RegisterStreamParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.RegisterStreamParams(v)
			if err != nil {
				return
			}
		}
	case *reward.RemoveStreamParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.RemoveStreamParams(v)
			if err != nil {
				return
			}
		}
	case *reward.SetDistributionParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.SetDistributionParams(v)
			if err != nil {
				return
			}
		}
	case *reward.SetSharesParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.SetSharesParams(v)
			if err != nil {
				return
			}
		}
	case *reward.ReplaceAddressParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ReplaceAddressParams(v)
			if err != nil {
				return
			}
		}
	case *reward.CancelPendingParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.CancelPendingParams(v)
			if err != nil {
				return
			}
		}
	case *reward.ClaimParams:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ClaimParams(v)
			if err != nil {
				return
			}
		}
	default:
		fmt.Println("Unknown type")
	}

	return
}
