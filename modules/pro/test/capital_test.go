package test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	probiz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/biz"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

func TestGetAndSave(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)
	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	redisRedis := redis.NewRedis(conf)
	biz := probiz.NewCapitalAnalysisBiz(db, agg, adapter, redisRedis)
	//address, err := biz.Agg.Address(context.Background(), chain.SmartAddress("f02181465"))
	//fmt.Println(address)
	//tipset, _ := agg.LatestTipset(context.Background())

	//biz.GetTransferTransaction(context.Background(), chain.Epoch(tipset[0].ID))
	//biz.GetALLTransaction(context.Background(), chain.Epoch(2849395).CurrentDay())
	//err = biz.LoadData()
	//require.NoError(t, err)
	resp, err := biz.EvaluateAddr(context.Background(), pro.EvaluateAddrRequest{
		Address: "f034689",
		Type:    "transaction_count",
	})
	require.NoError(t, err)
	marshal, err := json.Marshal(resp)
	require.NoError(t, err)
	fmt.Println(string(marshal))

	resp2, err := biz.CapitalAddrTransaction(context.Background(), pro.AddrTransactionRequest{
		Address: "f062294",
	})
	require.NoError(t, err)
	marshal2, err := json.Marshal(resp2)
	require.NoError(t, err)
	fmt.Println(string(marshal2))

}

func TestAddr(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)
	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)

	redisRedis := redis.NewRedis(conf)
	biz := probiz.NewCapitalAnalysisBiz(db, agg, adapter, redisRedis)
	address, err := biz.Agg.Address(context.Background(), chain.SmartAddress("f0440429"))
	require.NoError(t, err)
	fmt.Println(address)
}

func TestName(t *testing.T) {

}
