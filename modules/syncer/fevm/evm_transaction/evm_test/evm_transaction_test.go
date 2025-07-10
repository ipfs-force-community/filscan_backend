package evm_test

import (
	"github.com/gozelle/fs"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm_transaction"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"testing"
)

func TestTask(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)

	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)

	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)

	// 2683348, 3043348, 3044248
	start := chain.Epoch(3043345).Int64()
	end := chain.Epoch(3044345).Int64()

	s := syncer.NewSyncer(
		syncer.WithName("evm-transaction-syncer"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(&start),
		syncer.WithStopEpoch(&end),
		syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
		syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
		syncer.WithDry(true),
		syncer.WithTaskGroup(
			[]syncer.Task{
				evm_transaction.NewEvmTransactionTask(dal.NewEvmTransactionDal(db)), // 同步 EVM 类型 (FEVM合约) 地址
			},
		),
		syncer.WithCalculators(
			evm_transaction.NewEvmContractCalculator(dal.NewEvmTransactionDal(db)),
		),
		syncer.WithContextBuilder(injector.SetTracesBuilder),
	)
	err = s.Init()
	require.NoError(t, err)

	s.Run()
}
