package lotus_api

import (
	"context"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"testing"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func TestGetBlocks(t *testing.T) {

	glif, err := NewBasicAuthLotusApi("glif", "https://api.node.glif.io/rpc/v0")
	require.NoError(t, err)

	//tipset, err := glif.ChainHead(context.Background())
	//require.NoError(t, err)
	//require.True(t, len(tipset.Blocks()) > 0)
	//for _, v := range tipset.Blocks() {
	//	fmt.Println("block cid:", v.Cid())
	//}

	//epoch_test.go:13: 2470320
	//epoch_test.go:17: 2473200
	//epoch_test.go:21: 2476080
	addr1, err := address.NewFromString("f0441240")
	require.NoError(t, err)
	addr2, err := address.NewFromString("f01227975")
	require.NoError(t, err)
	addrs := []address.Address{addr1, addr2}
	for _, addr := range addrs {
		tipset1, err := glif.ChainGetTipSetByHeight(context.Background(), abi.ChainEpoch(chain.MustBuildEpochByDate("2023-01-02")), types.EmptyTSK)
		require.NoError(t, err)

		tipset2, err := glif.ChainGetTipSetByHeight(context.Background(), abi.ChainEpoch(chain.MustBuildEpochByDate("2023-01-03")), types.EmptyTSK)
		require.NoError(t, err)

		tipset3, err := glif.ChainGetTipSetByHeight(context.Background(), abi.ChainEpoch(chain.MustBuildEpochByDate("2023-01-04")), types.EmptyTSK)
		require.NoError(t, err)

		//t.Log(tipset1.Key())
		//t.Log(tipset2.Key())
		t3, _ := tipset3.Key().MarshalJSON()
		t.Log(string(t3))

		minerInfo, err := glif.StateMinerInfo(context.Background(), addr, types.EmptyTSK)
		require.NoError(t, err)

		res1, err := glif.StateMinerSectorCount(context.Background(), addr, tipset1.Key())
		require.NoError(t, err)

		res2, err := glif.StateMinerSectorCount(context.Background(), addr, tipset2.Key())
		require.NoError(t, err)

		res3, err := glif.StateMinerSectorCount(context.Background(), addr, tipset3.Key())
		require.NoError(t, err)

		t.Log("res3", res3.Live, res3.Active, minerInfo.SectorSize)
		t.Log("res2", res2.Live, res2.Active, minerInfo.SectorSize)
		t.Log("res1", res1.Live, res1.Active, minerInfo.SectorSize)
		t.Log(addr, "res3-res2", res3.Live-res2.Live, decimal.NewFromInt(int64(res3.Live-res2.Live)).Mul(decimal.NewFromInt(int64(minerInfo.SectorSize))).Div(decimal.NewFromFloat(1024*1024*1024*1024)))
		t.Log(addr, "res2-res1", res2.Live-res1.Live, decimal.NewFromInt(int64(res2.Live-res1.Live)).Mul(decimal.NewFromInt(int64(minerInfo.SectorSize))).Div(decimal.NewFromFloat(1024*1024*1024*1024)))
	}
}
