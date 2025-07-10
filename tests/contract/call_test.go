package contract

import (
	"fmt"
	"math/big"
	"path/filepath"
	"strings"
	"testing"
	
	_ "github.com/btcsuite/btcd/btcutil"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
)

func TestCall(t *testing.T) {
	
	//服务器地址
	conn, err := ethclient.Dial("https://api.node.glif.io/rpc/v0")
	if err != nil {
		fmt.Println("Dial err", err)
		return
	}
	defer conn.Close()
	
	root, err := fs.Lookup("tests/contract")
	require.NoError(t, err)
	
	t.Log(root)
	
	abiJson, err := fs.Read(filepath.Join(root, "ContractABI/IRegistrar.json"))
	require.NoError(t, err)
	
	abiInstance, _ := abi.JSON(strings.NewReader(string(abiJson)))
	
	contractAddr := common.HexToAddress("0xB8D7CA6a3253C418e52087693ca688d3257D70D1")
	
	contract := bind.NewBoundContract(contractAddr, abiInstance, conn, nil, nil)
	
	a := new(big.Int)
	tokenID := a.SetBytes(common.Hex2Bytes("717e7f3fe5fd104e633df25c7209da482b32884c7a616823633371381288fb38"))
	_ = tokenID
	var r []interface{}
	err = contract.Call(&bind.CallOpts{
		Pending:     false,
		From:        common.Address{},
		BlockNumber: nil,
		Context:     nil,
	}, &r, "ownerOf", tokenID)
	require.NoError(t, err)
	
	spew.Json(r)
}
