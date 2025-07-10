package v13

import (
	"bytes"
	"fmt"
	"reflect"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-bitfield"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/builtin/v13/datacap"
	"github.com/filecoin-project/go-state-types/builtin/v13/eam"
	initial "github.com/filecoin-project/go-state-types/builtin/v13/init"
	"github.com/filecoin-project/go-state-types/builtin/v13/market"
	"github.com/filecoin-project/go-state-types/builtin/v13/miner"
	"github.com/filecoin-project/go-state-types/builtin/v13/multisig"
	"github.com/filecoin-project/go-state-types/builtin/v13/power"
	"github.com/filecoin-project/go-state-types/builtin/v13/verifreg"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/message"
)

var newReturns = map[string]interface{}{
	"AddBalance":                 abi.EmptyValue{},
	"AddVerifiedClient":          abi.EmptyValue{},
	"AllowanceExported":          abi.TokenAmount{},
	"Approve":                    multisig.ApproveReturn{},
	"Cancel":                     abi.EmptyValue{},
	"CancelExported":             abi.EmptyValue{},
	"ChangeBeneficiary":          abi.EmptyValue{},
	"ChangeMultiaddrs":           abi.EmptyValue{},
	"ChangeOwnerAddress":         abi.EmptyValue{},
	"ChangePeerID":               abi.EmptyValue{},
	"ChangeWorkerAddress":        abi.EmptyValue{},
	"CompactPartitions":          abi.EmptyValue{},
	"CompactSectorNumbers":       abi.EmptyValue{},
	"ConfirmChangeWorkerAddress": abi.EmptyValue{},
	"Constructor":                abi.EmptyValue{},
	"CreateExternal":             eam.CreateExternalReturn{},
	"CreateMiner":                power.CreateMinerReturn{},
	"DeclareFaultsRecovered":     abi.EmptyValue{},
	"DisputeWindowedPoSt":        abi.EmptyValue{},
	"Exec":                       initial.ExecReturn{},
	"ExtendClaimTerms":           verifreg.ExtendClaimTermsReturn{},
	"ExtendSectorExpiration":     abi.EmptyValue{},
	"ExtendSectorExpiration2":    abi.EmptyValue{},
	"IncreaseAllowanceExported":  abi.TokenAmount{},
	"InvokeContract":             abi.CborBytes{},
	"PreCommitSector":            abi.EmptyValue{},
	"PreCommitSectorBatch":       abi.EmptyValue{},
	"Propose":                    multisig.ProposeReturn{},
	"ProveCommitAggregate":       abi.EmptyValue{},
	"ProveCommitSector":          abi.EmptyValue{},
	"ProveReplicaUpdates":        bitfield.BitField{},
	"PubkeyAddress":              address.Address{},
	"PublishStorageDeals":        market.PublishStorageDealsReturn{},
	"RemoveExpiredAllocations":   verifreg.RemoveExpiredAllocationsReturn{},
	"RepayDebt":                  abi.EmptyValue{},
	"ReportConsensusFault":       abi.EmptyValue{},
	"SubmitWindowedPoSt":         abi.EmptyValue{},
	"TerminateSectors":           miner.TerminateSectorsReturn{},
	"TransferFromExported":       datacap.TransferFromReturn{},
	"WithdrawBalance":            abi.TokenAmount{},
}

func DecodeMessageReturns(input interface{}, methodName string) (result interface{}, err error) {
	method := newReturns[methodName]
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
	case *abi.TokenAmount:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.TokenAmount(v)
			if err != nil {
				return
			}
		}
	case *multisig.ApproveReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ApproveReturn(v)
			if err != nil {
				return
			}
		}
	case *eam.CreateExternalReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.CreateExternalReturn(v)
			if err != nil {
				return
			}
		}
	case *power.CreateMinerReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.CreateMinerReturn(v)
			if err != nil {
				return
			}
		}
	case *initial.ExecReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ExecReturn(v)
			if err != nil {
				return
			}
		}
	case *verifreg.ExtendClaimTermsReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ExtendClaimTermsReturn(v)
			if err != nil {
				return
			}
		}
	case *multisig.ProposeReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.ProposeReturn(v)
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
	case *bitfield.BitField:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		result, err = message.DecodeBitField(*v)
		if err != nil {
			return
		}
	case *address.Address:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		result = v.String()
	case *market.PublishStorageDealsReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.PublishStorageDealsReturn(v)
			if err != nil {
				return
			}
		}
	case *verifreg.RemoveExpiredAllocationsReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.RemoveExpiredAllocationsReturn(v)
			if err != nil {
				return
			}
		}
	case *miner.TerminateSectorsReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.TerminateSectorsReturn(v)
			if err != nil {
				return
			}
		}
	case *datacap.TransferFromReturn:
		err = v.UnmarshalCBOR(bytes.NewReader(paramsByte))
		if err != nil {
			return
		}
		if v != nil {
			result, err = ConvertMessageType{}.TransferFromReturn(v)
			if err != nil {
				return
			}
		}
	default:
		fmt.Println("Unknown type")
	}

	return
}
