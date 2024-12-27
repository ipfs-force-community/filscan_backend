package luck_test

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task/luck"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"testing"
	
	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

func TestCalcLuck(t *testing.T) {
	
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)
	
	spew.Json(conf)
	
	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	
	calc := luck.NewCalculator(dal.NewLuckDal(db))
	
	lucks, err := calc.CalcMinersLuckRate(context.Background(), 3022560, "30d")
	require.NoError(t, err)
	
	for k, v := range lucks {
		t.Log(k, v.Round(4))
	}
	
}

func TestExport(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)
	
	spew.Json(conf)
	
	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	
	calc := luck.NewCalculator(dal.NewLuckDal(db))
	
	miner := "f01889512"
	exp, err := calc.Export(context.Background(), miner, 3022560, "30d")
	require.NoError(t, err)
	
	t.Logf("miner: %s", miner)
	t.Logf("luck: %s et: %s mt: %d", exp.Luck, exp.Et, exp.Mt)
	t.Logf("range:[%s,%s)", exp.Epochs.GteBegin, exp.Epochs.LtEnd)
	for _, v := range exp.Rows {
		t.Logf("%s  mPi: %s  tPi: %s tTi: %d eTi: %s", chain.Epoch(v.Epoch), v.Mpi, v.Tpi, v.Tti, v.Eti)
	}
	
	return
}
