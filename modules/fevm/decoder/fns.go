package decoder

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/pkg/errors"
	"github.com/wealdtech/go-ens/v3"
	
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
)

func NewFNS(ethClient *ethclient.Client) *FNS {
	return &FNS{client: NewEthClient(ethClient)}
}

var _ fevm.FNSAPI = (*FNS)(nil)

type FNS struct {
	client *EthClient
}

func (f FNS) FNSBalanceOf(contract fevm.Contract, address string) (domains int64, err error) {
	
	var res []interface{}
	
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "balanceOf", []*fevm.ContractParam{
		{
			Type:  "address",
			Value: address,
		},
	}, &res)
	if err != nil {
		return
	}
	
	domains = res[0].(*big.Int).Int64()
	
	return
}

func (f FNS) FNSTokenOfOwnerByIndex(contract fevm.Contract, address string, index uint64) (tokenID string, err error) {
	
	var res []interface{}
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "tokenOfOwnerByIndex", []*fevm.ContractParam{
		{
			Type:  "address",
			Value: address,
		},
		{
			Type:  "uint256",
			Value: big.NewInt(int64(index)),
		},
	}, &res)
	if err != nil {
		return
	}
	
	tokenID = fmt.Sprintf("0x%s", common.Bytes2Hex(res[0].(*big.Int).Bytes()))
	
	return
}

func (f FNS) FNSNode(contract fevm.Contract, addr string) (node string, err error) {
	var res []interface{}
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "node", []*fevm.ContractParam{
		{
			Type:  "address",
			Value: addr,
		},
	}, &res)
	if err != nil {
		return
	}
	
	a := res[0].([32]byte)
	node = fmt.Sprintf("0x%s", common.Bytes2Hex(a[:]))
	return
}

func (f FNS) FNSNodeName(contract fevm.Contract, node string) (name string, err error) {
	var res []interface{}
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "name", []*fevm.ContractParam{
		{
			Type:  "bytes32",
			Value: node,
		},
	}, &res)
	if err != nil {
		return
	}
	name = res[0].(string)
	
	return
}

func (f FNS) FNSGetNameByNode(contract fevm.Contract, node string) (name string, err error) {
	var res []interface{}
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "getNameByNode", []*fevm.ContractParam{
		{
			Type:  "bytes32",
			Value: node,
		},
	}, &res)
	if err != nil {
		return
	}
	name = res[0].(string)
	
	return
}

func (f FNS) FNSName(contract fevm.Contract, tokenID string) (name string, err error) {
	b := &big.Int{}
	b.SetBytes(common.Hex2Bytes(strings.TrimPrefix(tokenID, "0x")))
	
	var res []interface{}
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "name", []*fevm.ContractParam{
		{
			Type:  "big.Int",
			Value: b,
		},
	}, &res)
	if err != nil {
		return
	}
	
	name = res[0].(string)
	return
}

func (f FNS) FNSNameOf(contract fevm.Contract, tokenID string) (name string, err error) {
	var res []interface{}
	
	b := &big.Int{}
	b.SetBytes(common.Hex2Bytes(strings.TrimPrefix(tokenID, "0x")))
	
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "nameOf", []*fevm.ContractParam{
		{
			Type:  "big.Int",
			Value: b,
		},
	}, &res)
	if err != nil {
		return
	}
	
	name = res[0].(string)
	
	return
}

func (f FNS) FNSAvailable(contract fevm.Contract) (err error) {
	//TODO implement me
	panic("implement me")
}

func (f FNS) FNSTokenNode(domain string) (node string, err error) {
	name, err := ens.NameHash(domain)
	if err != nil {
		err = fmt.Errorf("namehash error: %s", err)
		return
	}
	
	node = fmt.Sprintf("0x%s", common.Bytes2Hex(name[:]))
	
	return
}

func (f FNS) FNSTokenId(domain string) (tokenId string, err error) {
	
	if domain == "" {
		return "", errors.New("empty domain")
	}
	
	domain, err = ens.NormaliseDomain(domain)
	if err != nil {
		return "", err
	}
	
	domain, err = ens.DomainPart(domain, 1)
	if err != nil {
		return "", err
	}
	
	labelHash, err := ens.LabelHash(domain)
	if err != nil {
		return "", err
	}
	
	hash := fmt.Sprintf("%#x", labelHash)
	r, ok := math.ParseBig256(hash)
	if !ok {
		err = fmt.Errorf("parseBig256: %s error", hash)
		return
	}
	tokenId = fmt.Sprintf("0x%s", common.Bytes2Hex(r.Bytes()))
	
	return
}

func (f FNS) FNSOwner(contract fevm.Contract, node string) (owner string, err error) {
	
	var res []interface{}
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "owner", []*fevm.ContractParam{
		{
			Type:  "bytes32",
			Value: node,
		},
	}, &res)
	if err != nil {
		err = fmt.Errorf("query fns owner error: %s", err)
		return
	}
	
	owner = res[0].(common.Address).String()
	
	return
}

func (f FNS) FNSOwnerOf(contract fevm.Contract, tokenID string) (address string, err error) {
	var res []interface{}
	
	b := &big.Int{}
	b.SetBytes(common.Hex2Bytes(strings.TrimPrefix(tokenID, "0x")))
	
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "ownerOf", []*fevm.ContractParam{
		{
			Type:  "big.Int",
			Value: b,
		},
	}, &res)
	if err != nil {
		return
	}
	
	address = res[0].(common.Address).String()
	
	return
}

func (f FNS) FNSNameExpires(contract fevm.Contract, name string) (expiredAt int64, err error) {
	var res []interface{}
	
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "nameExpires", []*fevm.ContractParam{
		{
			Type:  "string",
			Value: name,
		},
	}, &res)
	if err != nil {
		return
	}
	
	expiredAt = res[0].(*big.Int).Int64()
	
	return
}

func (f FNS) FNSFilAddr(contract fevm.Contract, node string) (str string, err error) {
	var res []interface{}
	
	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "addr0", []*fevm.ContractParam{
		{
			Type:  "bytes32",
			Value: node,
		},
		{
			Type:  "uint256",
			Value: big.NewInt(461),
		},
	}, &res)
	if err != nil {
		return
	}
	
	str = string(res[0].([]byte))
	
	return
}

//func (f FNS) FNSAddr(contract fevm.Contract, node string) (addr string, err error) {
//	var res []interface{}
//	err = f.client.Call(context.Background(), contract.ABI, contract.Address, "addr", []*fevm.ContractParam{
//		{
//			Type:  "bytes32",
//			Value: node,
//		},
//	}, &res)
//	if err != nil {
//		return
//	}
//	
//	spew.Json(res)
//	//a := res[0].([32]byte)
//	//node = fmt.Sprintf("0x%s", common.Bytes2Hex(a[:]))
//	
//	return
//}
