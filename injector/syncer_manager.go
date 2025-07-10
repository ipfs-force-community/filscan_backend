package injector

import (
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"

	"github.com/google/wire"
	"github.com/gozelle/mix"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	prosyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	change_actor_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/actor/change-actor-task"
	calc_change_actor_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-change-actor-task"
	calc_estimate_miner_gas "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-estimate-miner-gas"
	calc_fns_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-fns-task"
	calc_miner_acc_reward_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-acc-reward-task"
	calc_miner_agg_reward "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-agg-reward"
	calc_miner_owner_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task"
	rich_list_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/rich-list-task"
	capital_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/capital"
	builtin_actor_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/builtin-actor-task"
	deal_proposal_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/deal-proposal-task"
	message_count_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/message-count-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/reward_task"
	trace_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/trace-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/erc20"
	evm_transfer_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task"
	evm_transaction_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm_transaction"
	fns_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/fns-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/nft"
	miner_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/miner/miner-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
	"gorm.io/gorm"
)

var SyncerManagerSet = wire.NewSet(NewSyncerManager)

func SetTracesBuilder(ctx *syncer.Context) (err error) {

	if ctx.Empty() {
		_ = ctx.Datamap().Set(syncer.TracesTey, nil)
		return
	}

	now := time.Now()
	var traces []*londobell.TraceMessage

	defer func() {
		if c := time.Since(now); c > 5*time.Second {
			ctx.Debugf("高度:%s 获取 Traces 数量: %d 耗时: %s", ctx.Epoch(), len(traces), c)
		}
	}()

	epoch := ctx.Epoch()

	traces, err = ctx.Agg().Traces(ctx.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}

	if !ctx.Dry() {
		var tipset *londobell.EpochReply
		tipset, err = ctx.Adapter().Epoch(ctx.Context(), &epoch)
		if err != nil {
			return
		}
		if tipset.BlockCount > 0 && len(traces) == 0 {
			err = mix.Warnf("agg trace 未同步完毕")
			return
		}
	}

	err = ctx.Datamap().Set(syncer.TracesTey, traces)
	if err != nil {
		return
	}

	return
}

func newSyncer(conf *config.Config, db *gorm.DB, agg londobell.Agg, adapter londobell.Adapter, options ...syncer.Option) *syncer.Syncer {

	s := syncer.NewSyncer(
		append([]syncer.Option{
			syncer.WithDB(db),
			syncer.WithLondobellAgg(agg),
			syncer.WithLondobellAdapter(adapter),
			syncer.WithInitEpoch(conf.InitEpoch),
			syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
			syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
		}, options...)...,
	)
	return s
}

func StateChecker(ctx *syncer.Context) error {
	epoch, err := ctx.Agg().StateFinalHeight(ctx.Context())
	if err != nil {
		return err
	}
	if epoch == nil {
		return mix.Warnf("StateFinalHeight is nil")
	}
	if *epoch < ctx.Epoch() {
		return mix.Warnf("StateFinalHeight: %s 未到, 还差: %d", *epoch, ctx.Epoch()-*epoch)
	}
	return nil
}

func ProChecker(ctx *syncer.Context) error {
	e := StateChecker(ctx)
	if e != nil {
		return e
	}
	epoch, err := ctx.Agg().FinalHeight(ctx.Context())
	if err != nil {
		return err
	}
	if epoch == nil {
		return mix.Warnf("FinalHeight is nil")
	}
	if *epoch < ctx.Epoch() {
		return mix.Warnf("FinalHeight 高度: %s 未到, 还差: %d", *epoch, ctx.Epoch()-*epoch)
	}
	return nil
}

func NewSyncerManager(conf *config.Config, db *gorm.DB, agg londobell.Agg, minerAgg londobell.MinerAgg, adapter londobell.Adapter, abiDecoder fevm.ABIDecoderAPI, redis *redis.Redis) *syncer.Manager {

	enable := map[string]struct{}{}
	for _, v := range conf.Syncer.EnableSyncers {
		enable[v] = struct{}{}
	}

	evmStartEpoch := int64(message_detail.HYGGE)

	if conf.ABINode == nil {
		panic("should config abi node")
	}
	abiNodeToken := ""
	if conf.ABINodeToken != nil {
		abiNodeToken = *conf.ABINodeToken
	}
	node, err := lotus_api.NewBasicAuthLotusApi("self", *conf.ABINode, lotus_api.WithAuth("", abiNodeToken))
	if err != nil {
		panic("should config abi node")
	}
	//opengateInitEpoch := int64(2685202)

	m := syncer.NewManager(enable, []*syncer.Syncer{
		// 链数据指标同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.ChainSyncer),
			syncer.WithContextBuilder(SetTracesBuilder),
			syncer.WithTaskGroup(
				[]syncer.Task{
					trace_task.NewTraceTask(db, adapter),                           // 解析 Trace，记录高度手续费消耗等
					builtin_actor_task.NewBaselineTask(dal.NewBaseLineTaskDal(db)), // 同步内置 Actor 状态, 爆块奖励依赖此任务
					reward_task.NewMinerRewardTask(dal.NewRewardTaskDal(db)),       // 记录 Miner爆块奖励 及 WinCount
				},
				[]syncer.Task{
					deal_proposal_task.NewDealProposalTask(dal.NewDealProposalTaskDal(db)), // 记录订单提案
				},
				[]syncer.Task{
					message_count_task.NewMessageCountTask(dal.NewMessageCountTaskDal(db)), // 同步消息走势
				},
			),
			syncer.WithCalculators(
				calc_miner_acc_reward_task.NewCalcMinerAccRewardTask(dal.NewRewardTaskDal(db)), // 计算 Miner 奖励统计值，依赖历史高度汇总值
				calc_estimate_miner_gas.NewCalEstimateMinerGas(dal.NewSyncerTraceTaskDal(db)),  // 通过 BaselineTask 任务的高度值，计算当下高度的 Miner 消耗预估值
				calc_miner_agg_reward.NewCalcMinerAggReward(dal.NewRewardTaskDal(db)),          // 计算 Miner 的历史统计值，提供 Pro 使用
			),
		),

		// Miner 同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.MinerSyncer),
			syncer.WithContextBuilder(StateChecker),
			syncer.WithTaskGroup(
				[]syncer.Task{
					miner_task.NewMinerInfoTask(conf, dal.NewSyncEpochGetterDal(db), dal.NewMinerTaskDal(db)), // 记录 MinerInfos
				},
			),
			syncer.WithCalculators(
				calc_miner_owner_task.NewCalcMinerOwnerTask(conf, db),                  // 计算 Miner、Owner 统计汇总，依赖历史高度汇总值
				rich_list_task.NewActorBalanceTask(db, dal.NewActorBalanceTaskDal(db)), // 同步前1000名账号的余额变化, 依赖高度顺序
			),
		),

		// Pro 同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.ProSyncer),
			syncer.WithLondobellMinerAgg(minerAgg),
			syncer.WithContextBuilder(ProChecker),
			syncer.WithCalculators(
				prosyncer.NewProCalculator(db, prosyncer.ProCalculatorParams{
					Store:              true,
					DisableSyncBalance: conf.Pro.DisableSyncBalance,
					DisableSyncFund:    conf.Pro.DisableSyncFund,
					DisableSyncInfo:    conf.Pro.DisableSyncInfo,
				}), // 汇总 Pro 需要的 Miner 数据
			),
		),

		// 近期转账同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.CapitalSyncer),
			syncer.WithRedis(redis),
			syncer.WithTaskGroup(
				[]syncer.Task{
					capital_task.NewCapitalTask(conf, agg, redis),
				},
			),
		),

		// ChangeActor 同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.ActorSyncer),
			syncer.WithTaskGroup(
				[]syncer.Task{
					change_actor_task.NewChangeActorTask(dal.NewChangeActorTaskDal(db)), // 变动 Actor 记录
				},
			),
			syncer.WithCalculators(
				calc_change_actor_task.NewCalcChangeActorTask(dal.NewChangeActorTaskDal(db)), // 变动 Actor 记录计算任务
			),
		),

		// Erc20 数据同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.Erc20Syncer),
			syncer.WithContextBuilder(SetTracesBuilder),
			syncer.WithTaskGroup(
				[]syncer.Task{
					erc20.NewERC20Task(abiDecoder, dal.NewERC20Dal(db), node), // 同步 ERC 20
				},
			),
		),

		// Defi 数据同步
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.DefiSycner),
			syncer.WithContextBuilder(SetTracesBuilder),
			syncer.WithTaskGroup(
				[]syncer.Task{
					defi_task.NewDefiDashboardTask(abiDecoder, dal.NewDefiDashboardDal(db, dal.NewERC20Dal(db)), node),
				},
			),
		),

		//  Fns 数据同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.FnsSyncer),
			syncer.WithContextBuilder(SetTracesBuilder),
			//syncer.WithInitEpoch(&opengateInitEpoch),
			//syncer.WithSkipSyncEpoch(2964768),
			//syncer.WithEpochsThreshold(1200),
			//syncer.WithEpochsChunk(100),
			syncer.WithTaskGroup(
				[]syncer.Task{
					fns_task.NewFnsTask(dal.NewFEvmDal(db), dal.NewFnsSaverDal(db)), // 同步 FNS 域名事件
				},
			),
			syncer.WithCalculators(
				calc_fns_task.NewCalcFnsTask(abiDecoder, dal.NewFnsSaverDal(db)), // 解析事件还原 Token
			),
		),

		// evm transfer 数据同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.EvmContractSyncer),
			syncer.WithContextBuilder(SetTracesBuilder),
			//syncer.WithInitEpoch(),
			syncer.WithTaskGroup(
				[]syncer.Task{
					evm_transfer_task.NewEVMTransferTask(dal.NewEVMTransferDal(db)), // 同步 EVM 类型 (FEVM合约) 地址
				},
			),
		),

		// nft 同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.NFTSyncer),
			syncer.WithInitEpoch(nil), // 上线时从当前高度开始同步，历史数据自动补
			syncer.WithContextBuilder(SetTracesBuilder),
			syncer.WithTaskGroup(
				[]syncer.Task{
					nft.NewNFTTask(db, abiDecoder),
				},
			),
			syncer.WithCalculators(
				nft.NewNFTCalculator(db, abiDecoder),
			),
		),

		// evm transaction 数据同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.EvmSyncer),
			syncer.WithInitEpoch(&evmStartEpoch),
			syncer.WithContextBuilder(SetTracesBuilder),
			syncer.WithTaskGroup(
				[]syncer.Task{
					evm_transaction_task.NewEvmTransactionTask(dal.NewEvmTransactionDal(db)), // 同步 EVM 类型 (FEVM合约) 地址
				},
			),
		),

		// sector 数据同步器
		newSyncer(conf, db, agg, adapter,
			syncer.WithName(syncer.SectorSyncer),
			syncer.WithInitEpoch(nil),
			syncer.WithContextBuilder(ProChecker),
			syncer.WithTaskGroup(
				[]syncer.Task{
					prosyncer.NewSectorTask(db, true), // 同步每日 Sector
				},
			),
		),
	})

	return m
}
