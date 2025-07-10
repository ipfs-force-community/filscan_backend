package decoder

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
)

func NewEthClient(client *ethclient.Client) *EthClient {
	return &EthClient{client: client}
}

type EthClient struct {
	client *ethclient.Client
}

func (a EthClient) BlockNumber(ctx context.Context) (uint64, error) {
	return a.client.BlockNumber(ctx)
}

func (a EthClient) Call(ctx context.Context, abiJson []byte, contractEthAddr string, method string, params []*fevm.ContractParam, result *[]interface{}) (err error) {
	
	abiInstance, err := abi.JSON(strings.NewReader(string(abiJson)))
	if err != nil {
		return
	}
	
	contract := bind.NewBoundContract(common.HexToAddress(contractEthAddr), abiInstance, a.client, nil, nil)
	
	var _params []interface{}
	
	for _, v := range params {
		switch v.Type {
		case "big.Int", "uint256":
			vv := &big.Int{}
			c, ok := v.Value.(string)
			if ok {
				bs := common.Hex2Bytes(strings.TrimPrefix(c, "0x"))
				vv.SetBytes(bs)
				_params = append(_params, vv)
				continue
			}
			vv, ok = v.Value.(*big.Int)
			if !ok {
				err = fmt.Errorf("contract method param type: big.Int value expect *big.Int")
				return
			}
			_params = append(_params, vv)
		case "string":
			s, ok := v.Value.(string)
			if !ok {
				err = fmt.Errorf("contract method param type: string value expect string")
				return
			}
			_params = append(_params, s)
		case "address":
			s, ok := v.Value.(string)
			if !ok {
				err = fmt.Errorf("contract method param type: address value expect string")
				return
			}
			addr := common.HexToAddress(strings.TrimPrefix(s, "0x"))
			_params = append(_params, addr)
		case "bytes32":
			s, ok := v.Value.(string)
			if !ok {
				err = fmt.Errorf("contract method param type: bytes32 value expect string")
				return
			}
			bs := common.Hex2Bytes(strings.TrimPrefix(s, "0x"))
			var bs32 [32]byte
			for ii := 0; ii < 32; ii++ {
				bs32[ii] = bs[ii]
			}
			_params = append(_params, bs32)
		default:
			err = fmt.Errorf("unsupport contract parmemter type: %s", v.Type)
			return
		}
	}
	
	err = contract.Call(&bind.CallOpts{
		Pending:     false,
		From:        common.Address{},
		BlockNumber: nil,
		Context:     ctx,
	}, result, method, _params...)
	if err != nil {
		return
	}
	
	return
}
