package fevm

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"math/big"
)

type ContractAPI interface {
	ChainHead() (epoch chain.Epoch, err error)
	LookupMethod(abiJson, input []byte) (name string, err error)
	DecodeEventHexAddress(addr string) (r string, err error)
	DecodeMethodInput(abiJson, input []byte) (reply *DecodeMethodInputReply, err error)
	LookupEvent(abiJson, eventHash []byte) (name string, err error)
	DecodeEventInput(abiJson, eventHash, data []byte) (reply *DecodeEventInputReply, err error)
	ParseABISignatures(abiJson []byte) (parsed []*ABISignature, err error)
	DetectContractProtocol(bytecode []byte) (res Protocols, err error)
	HexToEthAddress(hexStr string) (ethAddr string, err error)
	HexToBigInt(hexStr string) (r *big.Int, err error)
	CallContract(abiJson []byte, contractAddress, method string, params []*ContractParam) (result []interface{}, err error)
}

type Protocols struct {
	Erc20          bool
	Erc721         bool
	Erc721MetaData bool
	Pair           bool
}
