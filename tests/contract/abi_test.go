package contract

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gozelle/fs"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"path/filepath"
	"testing"
)

func TestCalldata(t *testing.T) {
	
	root, err := fs.Lookup("tests/contract")
	require.NoError(t, err)
	
	t.Log(root)
	
	abiStr, err := fs.Read(filepath.Join(root, "ContractABI/IRegistrarController.json"))
	require.NoError(t, err)
	
	// 合约调用的输入参数（16 进制格式）
	inputHex := "5692a2cf00000000000000000000000000000000000000000000000000000000000000c00000000000000000000000008c3ecf71bba1356c6988ef0d71860f05c2fc41950000000000000000000000000000000000000000000000000000000001e13380000000000000000000000000e1b0c92be9c3855835ddbd0ef4c80dee8fe41f60000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000009747578696e6773756e00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	
	// 解析 ABI
	parsed, err := abi.JSON(bytes.NewReader(abiStr))
	if err != nil {
		panic(err)
	}
	
	// 解码输入参数
	inputBytes, err := hex.DecodeString(inputHex)
	if err != nil {
		panic(err)
	}
	m, err := parsed.MethodById(inputBytes)
	require.NoError(t, err)
	t.Log(m.String())
	t.Log(m.Name, hex.EncodeToString(m.ID))
	
	values, err := m.Inputs.Unpack(inputBytes[4:])
	if err != nil {
		panic(err)
	}
	t.Log(values)
	
}

func TestEvent(t *testing.T) {
	
	root, err := fs.Lookup("tests/contract")
	require.NoError(t, err)
	
	t.Log(root)
	
	abiJson, err := fs.Read(filepath.Join(root, "ContractABI/PublicResolver.json"))
	require.NoError(t, err)
	
	// 合约调用的输入参数（16 进制格式）
	
	// 解析 ABI
	parsed, err := abi.JSON(bytes.NewReader(abiJson))
	if err != nil {
		panic(err)
	}
	
	eventID := common.HexToHash("335721b01866dc23fbee8b6b2c7b1e14d6f05c28cd35a2c934239f94095602a0")
	m, err := parsed.EventByID(eventID)
	require.NoError(t, err)
	t.Log(m.String())
	t.Log(m.Name)
	
	values, err := m.Inputs.Unpack(common.Hex2Bytes("000000000000000000000000e1b0c92be9c3855835ddbd0ef4c80dee8fe41f60"))
	if err != nil {
		panic(err)
	}
	t.Log(values)
	
}

func TestAbiDecodeEvent(t *testing.T) {
	
	root, err := fs.Lookup("tests/contract")
	require.NoError(t, err)
	
	t.Log(root)
	
	abiJson, err := fs.Read(filepath.Join(root, "ContractABI/FNSRegistry.json"))
	require.NoError(t, err)
	
	abiDecoder := getAbiDecoder(t, getConf(t))
	
	eventID := common.HexToHash("335721b01866dc23fbee8b6b2c7b1e14d6f05c28cd35a2c934239f94095602a0")
	
	r, err := abiDecoder.DecodeEventInput(abiJson, eventID.Bytes(), common.Hex2Bytes("000000000000000000000000e1b0c92be9c3855835ddbd0ef4c80dee8fe41f60"))
	require.NoError(t, err)
	
	t.Log(r.RawName)
	t.Log(r.Values)
}

func getConf(t *testing.T) *config.Config {
	root, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(root, conf)
	require.NoError(t, err)
	return conf
}

func getAbiDecoder(t *testing.T, conf *config.Config) fevm.ABIDecoderAPI {
	abiDecoder, err := fevm.NewAbiDecoderClient(*conf.ABIDecoderRPC)
	require.NoError(t, err)
	return abiDecoder
}

func TestDecodeMethodInput(t *testing.T) {
	
	root, err := fs.Lookup("tests/contract")
	require.NoError(t, err)
	abiJson, err := fs.Read(filepath.Join(root, "ContractABI/IRegistrarController.json"))
	require.NoError(t, err)
	
	t.Log(string(abiJson))
	
	// 合约调用的输入参数（16 进制格式）
	inputHex := "5692a2cf00000000000000000000000000000000000000000000000000000000000000c00000000000000000000000008c3ecf71bba1356c6988ef0d71860f05c2fc41950000000000000000000000000000000000000000000000000000000001e13380000000000000000000000000e1b0c92be9c3855835ddbd0ef4c80dee8fe41f60000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000009747578696e6773756e00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"
	
	inputBytes, err := hex.DecodeString(inputHex)
	require.NoError(t, err)
	
	abiDecoder := getAbiDecoder(t, getConf(t))
	
	r, err := abiDecoder.DecodeMethodInput(abiJson, inputBytes)
	require.NoError(t, err)
	
	t.Log(r.RawName)
	t.Log(r.Values)
	
	t.Log(decimal.NewFromFloat(r.Values[2].(float64)).String())
}

func TestDecodeMethodName(t *testing.T) {
	
	a := common.Hex2Bytes("ddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
	for _, c := range a[:4] {
		t.Logf("%c", c)
	}
	
}

func TestFIlAddress(t *testing.T) {
	
	a, err := base64.StdEncoding.DecodeString("5aSq5LiA55Sf5rC0")
	require.NoError(t, err)
	
	//t.Log(common.BytesToAddress(a))
	t.Log(string(a))
	
}
