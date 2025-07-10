package syncertest

import (
	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	prosyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"testing"
)

func TestSectorTask(t *testing.T) {
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
	
	initEpoch := chain.MustBuildEpochByTime("2023-09-08 00:00:00").Int64()
	spew.Json(initEpoch)
	//latest, err := agg.FinalHeight(context.Background())
	//require.NoError(t, err)
	//spew.Json(latest)
	
	task := prosyncer.NewSectorTask(db, false)
	//task.TestMiners = "f01975316"
	s := syncer.NewSyncer(
		syncer.WithName("test-pro-syncer"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellMinerAgg(minerAgg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(&initEpoch),
		syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
		syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
		syncer.WithDry(true),
		syncer.WithTaskGroup([]syncer.Task{
			task,
		}),
	)
	err = s.Init()
	require.NoError(t, err)
	
	s.Run()
}
