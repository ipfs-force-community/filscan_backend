package nft_test

import (
	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/nft"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"testing"
)

func TestTask(t *testing.T) {
	f, err := fs.Lookup("configs/ty.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)
	
	spew.Json(conf)
	
	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)
	
	abiDecoder, err := injector.NewAbiDecoderClient(conf)
	require.NoError(t, err)
	
	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)
	
	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	
	initEpoch := chain.MustBuildEpochByTime("2023-07-25 12:28:00").Int64()
	
	s := syncer.NewSyncer(
		syncer.WithName("nft"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(&initEpoch),
		syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
		syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
		//syncer.WithDry(true),
		syncer.WithTaskGroup(
			[]syncer.Task{
				nft.NewNFTTask(db, abiDecoder),
			},
		),
		syncer.WithCalculators(
			nft.NewNFTCalculator(db, abiDecoder),
		),
		syncer.WithContextBuilder(injector.SetTracesBuilder),
	)
	err = s.Init()
	require.NoError(t, err)
	
	s.Run()
}
