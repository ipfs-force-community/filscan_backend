package evm_signature

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
)

type EvmSignatureConvertor struct {
}

func (e EvmSignatureConvertor) EventSignatureAPIToEventSignatureDB(input []*EventSignature) (result []*po.EvmEventSignature) {
	for _, signature := range input {
		result = append(result, &po.EvmEventSignature{
			ID:            signature.Id,
			TextSignature: signature.TextSignature,
			HexSignature:  signature.HexSignature,
			CreatedAt:     signature.CreatedAt,
		})
	}

	return
}
