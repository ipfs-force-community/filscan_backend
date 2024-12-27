package erc20

import (
	"context"
	"fmt"
	"testing"

	"github.com/filecoin-project/lotus/chain/types/ethtypes"
	"github.com/gozelle/fs"
	"github.com/gozelle/testify/require"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

func TestIsPairFunc(t *testing.T) {
	contractID := "0x45680718f6bdb7ec3a7df7d61587ac7c3fb49d50"
	ethAddr, err := ethtypes.ParseEthAddress(contractID)
	require.NoError(t, err)
	node, err := lotus_api.NewBasicAuthLotusApi("glif", "https://api.node.glif.io/rpc/v0")
	require.NoError(t, err)
	predefinedBlock := "latest"
	ebytes, err := node.EthGetCode(context.TODO(), ethAddr, ethtypes.EthBlockNumberOrHash{
		PredefinedBlock: &predefinedBlock,
	})
	require.NoError(t, err)

	decoder := getAbiDecoder(t, getConf(t))
	res, err := decoder.DetectContractProtocol(ebytes)
	fmt.Println(res)
	require.NoError(t, err)
	require.True(t, res.Pair)
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
