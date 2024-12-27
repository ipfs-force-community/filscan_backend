package impltest

import (
	"context"
	"github.com/golang-module/carbon/v2"
	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	mergerimpl "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/merger/impl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"testing"
	"time"
)

func TestMerger(t *testing.T) {
	
	f, err := fs.Lookup("configs/ty.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)
	
	spew.Json(conf)
	
	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	
	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)
	
	minerAgg, err := injector.NewLondobellMinerAgg(conf)
	require.NoError(t, err)
	
	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	_ = db
	
	date, err := chain.NewDate(carbon.Shanghai, time.Now().AddDate(0, 0, -2).Format(carbon.DateLayout))
	require.NoError(t, err)
	
	m := mergerimpl.NewMergerImpl(db, adapter, agg, minerAgg)
	
	//testMinersInfoStats(t, m, date)
	testMinersFundStats(t, m, date)
	//testMinersBalanceStats(t, m, date)
	//testMinersRewardStats(t, m, date)
	//testMinersPowerStats(t, m, date)
	//testMinersSectorStats(t, m, date)
}

func testMinersInfoStats(t *testing.T, m *mergerimpl.MergerImpl, date chain.Date) {
	
	update, summary, infos, err := m.MinersInfos(context.Background(), []chain.SmartAddress{"f0334455", "f02200472", "f012345", "f012334"}, date)
	require.NoError(t, err)
	
	spew.Json(update)
	spew.Json(summary)
	spew.Json(infos)
}

func testMinersFundStats(t *testing.T, m *mergerimpl.MergerImpl, date chain.Date) {
	
	update, res, err := m.MinersFundStats(context.Background(), []chain.SmartAddress{"f02104858"}, chain.NewDateLCRCRange(date, date))
	require.NoError(t, err)
	spew.Json(update)
	spew.Json(res)
}

func testMinersBalanceStats(t *testing.T, m *mergerimpl.MergerImpl, date chain.Date) {
	update, res, err := m.MinersBalanceStats(context.Background(), []chain.SmartAddress{"f02104858"}, date)
	require.NoError(t, err)
	spew.Json(update)
	spew.Json(res)
}

func testMinersSectorStats(t *testing.T, m *mergerimpl.MergerImpl, date chain.Date) {
	update, res, err := m.MinersSectorStats(context.Background(), []chain.SmartAddress{"f02200472"})
	require.NoError(t, err)
	spew.Json(update)
	spew.Json(res)
}

func testMinersRewardStats(t *testing.T, m *mergerimpl.MergerImpl, date chain.Date) {
	update, res, err := m.MinersRewardStats(context.Background(), []chain.SmartAddress{"f01889512"}, chain.NewDateLCRCRange(date, date))
	require.NoError(t, err)
	spew.Json(update)
	spew.Json(res)
}

func testMinersPowerStats(t *testing.T, m *mergerimpl.MergerImpl, date chain.Date) {
	update, res, err := m.MinersPowerStats(context.Background(), []chain.SmartAddress{"f0334455", "f02200472", "f012345", "f012334"}, chain.NewDateLCRCRange(date, date))
	require.NoError(t, err)
	spew.Json(update)
	spew.Json(res)
}
