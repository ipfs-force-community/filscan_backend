package syncer_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/gozelle/fs"
	"github.com/gozelle/pointer"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_fns_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-fns-task"
	calc_miner_acc_reward_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-acc-reward-task"
	calc_miner_owner_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task"
	capital_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/capital"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/erc20"
	evm_transfer_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task"
	fns_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/fns-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/nft"
	miner_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/miner/miner-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/test"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

func TestErc20Syncer(t *testing.T) {

	f, err := fs.Lookup("configs/local.toml")
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
	node, err := lotus_api.NewBasicAuthLotusApi("test", *conf.ABINode)
	require.NoError(t, err)
	s := syncer.NewSyncer(
		syncer.WithName("test-erc20"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(pointer.ToInt64(3466168)),
		//syncer.WithInitEpoch(pointer.ToInt64(2685202)),      // 初始高度 >=
		//syncer.WithStopEpoch(pointer.ToInt64(2685202+2880)), // 截止高度：<=
		//syncer.WithDry(true),                                // 不检查链一直性
		syncer.WithTaskGroup(
			[]syncer.Task{
				erc20.NewERC20Task(abiDecoder, dal.NewERC20Dal(db), node), // 同步 ERC 20
				//test.NewTask(),
			},
		),
		syncer.WithCalculators(),
		syncer.WithContextBuilder(injector.SetTracesBuilder),
	)
	err = s.Init()
	require.NoError(t, err)

	s.Run()
}

func TestFnsSyncer(t *testing.T) {

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

	s := syncer.NewSyncer(
		syncer.WithName("test-fns2"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(pointer.ToInt64(chain.CurrentEpoch().Int64()-2880)),
		syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
		syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
		syncer.WithTaskGroup(
			[]syncer.Task{
				fns_task.NewFnsTask(dal.NewFEvmDal(db), dal.NewFnsSaverDal(db)), // 同步 FNS 域名事件
			},
		),
		syncer.WithCalculators(
			calc_fns_task.NewCalcFnsTask(abiDecoder, dal.NewFnsSaverDal(db)), // 解析事件还原 Token
		),
		syncer.WithContextBuilder(injector.SetTracesBuilder),
	)
	err = s.Init()
	require.NoError(t, err)

	s.Run()
}

func TestCapitalSyncer(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	adapter, err := injector.NewLondobellAdapter(conf)
	require.NoError(t, err)

	redisRedis := redis.NewRedis(conf)

	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)

	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)

	s := syncer.NewSyncer(
		syncer.WithName("test-capital"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		//syncer.WithInitEpoch(pointer.ToInt64(2849395)),
		syncer.WithTaskGroup(
			[]syncer.Task{
				capital_task.NewCapitalTask(conf, agg, redisRedis), // 同步
			},
		),
	)
	err = s.Init()
	require.NoError(t, err)

	s.Run()
}

func TestNFTSyncer(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
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
	// nft 同步器
	s := syncer.NewSyncer(
		syncer.WithName("test-nft"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithContextBuilder(injector.SetTracesBuilder),
		syncer.WithInitEpoch(pointer.ToInt64(3642311)),
		syncer.WithTaskGroup(
			[]syncer.Task{
				nft.NewNFTTask(db, abiDecoder),
			},
		),
		syncer.WithCalculators(
			nft.NewNFTCalculator(db, abiDecoder),
		),
	)

	err = s.Init()
	require.NoError(t, err)

	s.Run()
}

func TestCapital(t *testing.T) {
	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	agg, err := injector.NewLondobellAgg(conf)
	require.NoError(t, err)
	redisRedis := redis.NewRedis(conf)
	task := capital_task.NewCapitalTask(conf, agg, redisRedis) // 同步
	ctx := &syncer.Context{}
	err = task.Exec(ctx)
	require.NoError(t, err)
}

func TestMinerSyncer(t *testing.T) {

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
	task := miner_task.NewMinerInfoTask(conf, dal.NewSyncEpochGetterDal(db), dal.NewMinerTaskDal(db))
	task.GapScan = true
	s := syncer.NewSyncer(
		syncer.WithName("test-miner"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(pointer.ToInt64(3688679)),
		syncer.WithTaskGroup(
			[]syncer.Task{
				task, // 记录 MinerInfos
			},
		),
		syncer.WithCalculators(
			calc_miner_owner_task.NewCalcMinerOwnerTask(conf, db), // 计算 Miner、Owner 统计汇总，依赖历史高度汇总值
		),
	)
	err = s.Init()
	require.NoError(t, err)

	s.Run()
}

func TestChainSyncer(t *testing.T) {

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
	s := syncer.NewSyncer(
		syncer.WithName("test-chain"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(pointer.ToInt64(2297520)),
		syncer.WithStopEpoch(pointer.ToInt64(3236399)),
		syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
		syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
		syncer.WithTaskGroup(
			[]syncer.Task{
				//trace_task.NewTraceTask(db, adapter),                           // 解析 Trace，记录高度手续费消耗等
				//builtin_actor_task.NewBaselineTask(dal.NewBaseLineTaskDal(db)), // 同步内置 Actor 状态, 爆块奖励依赖此任务
				//reward_task.NewMinerRewardTask(dal.NewRewardTaskDal(db)),       // 记录 Miner爆块奖励 及 WinCount
				//test.NewTask(),
			},
		),
		syncer.WithCalculators(
			calc_miner_acc_reward_task.NewCalcMinerAccRewardTask(dal.NewRewardTaskDal(db)), // 计算 Miner 奖励统计值，依赖历史高度汇总值
			//calc_estimate_miner_gas.NewCalEstimateMinerGas(dal.NewSyncerTraceTaskDal(db)),  // 测试 Miner 封装预估
		),
		syncer.WithContextBuilder(injector.SetTracesBuilder),
	)
	//err = s.Init()
	//require.NoError(t, err)
	//err = s.Rollback(0, 2981399)
	//require.NoError(t, err)

	err = s.Init()
	require.NoError(t, err)
	s.Run()
}

func TestCheckConsistency(t *testing.T) {

	f, err := fs.Lookup("configs/local.toml")

	//f, err := fs.Lookup("configs/ty.toml")
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

	s := syncer.NewSyncer(
		syncer.WithName("fns"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(conf.InitEpoch),
		syncer.WithTaskGroup(
			[]syncer.Task{
				test.NewTask(),
			},
		),
	)
	err = s.Init()
	require.NoError(t, err)

	_, err = s.CheckConsistency(2983127-901, 2983127)
	require.NoError(t, err)
	_, err = s.CheckConsistency(2983127-901, 2983127)
	require.NoError(t, err)

	t.Log("一致性检查通过")
}

func TestEvmTransfer(t *testing.T) {

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
	initEpoch := int64(3299123)
	s := syncer.NewSyncer(
		syncer.WithContextBuilder(injector.SetTracesBuilder),
		syncer.WithName("test-evm-transfer10"),
		syncer.WithDB(db),
		syncer.WithLondobellAgg(agg),
		syncer.WithLondobellAdapter(adapter),
		syncer.WithInitEpoch(&initEpoch),
		syncer.WithEpochsChunk(1),
		syncer.WithEpochsThreshold(1),
		syncer.WithTaskGroup(
			[]syncer.Task{
				evm_transfer_task.NewEVMTransferTask(dal.NewEVMTransferDal(db)), // 同步 EVM 类型 (FEVM合约) 地址
			},
		),
	)
	err = s.Init()
	require.NoError(t, err)
	err = s.Rollback(0, 3299153) //这里实际上是end是同步到了哪儿， rollEpoch是哪里发生了冲突
	if err != nil {              //todo 不要随便回滚，仅测试使用
		return
	}
	s.Run()
}

func TestEvmTransferStat(t *testing.T) {

	f, err := fs.Lookup("configs/local.toml")
	require.NoError(t, err)
	conf := new(config.Config)
	err = _config.UnmarshalConfigFile(f, conf)
	require.NoError(t, err)

	spew.Json(conf)

	db, _, err := injector.NewGormDB(conf)
	require.NoError(t, err)
	task := evm_transfer_task.NewEVMTransferTask(dal.NewEVMTransferDal(db)) // 同步 EVM 类型 (FEVM合约) 地址

	stats, err := task.HandlerEvmTransferStats(context.Background(), 3010320)
	require.NoError(t, err)

	err = task.SaveEvmTransferStats(context.Background(), stats)
	require.NoError(t, err)

	fmt.Printf("stats: %v", stats)
}
