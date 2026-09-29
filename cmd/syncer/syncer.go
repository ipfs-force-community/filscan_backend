package main

import (
	"context"
	"fmt"
	"os"

	"github.com/gozelle/gin"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/mix"
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/fullactors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/ipquery"
	price_syncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/price-syncer"
	procmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/cmd"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	syncer_api "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/api"
	actoractionscmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-change-actor-task/cmd"
	minergasestimatecmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-estimate-miner-gas/cmd"
	mineraccrewardcmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-acc-reward-task/cmd"
	mineraggrewardcmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-agg-reward/cmd"
	baselineactorscmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/builtin-actor-task/cmd"
	minerrewardscmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/reward_task/cmd"
	minergasfeecmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/trace-task/cmd"
	erc20cmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/erc20/cmd"
	evmtransfercmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task/cmd"
	fnscmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/fns-task/cmd"
	nftcmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/nft/cmd"
	minertaskcmd "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/miner/miner-task/cmd"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string = "syncer-local"
	// Version is the version of the compiled software.
	Version string

	id, _ = os.Hostname()
)

var (
	rootCmd    *cobra.Command
	configFile string
)

var (
	log = logging.NewLogger("main")
)

func init() {
	rootCmd = &cobra.Command{
		Use:     "filscan-syncer [-c|--config /path/to/config.toml]",
		Short:   Name,
		Run:     runCmd,
		Args:    cobra.ExactArgs(0),
		Version: Version,
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径")
	rootCmd.Flags().SortFlags = false
	_ = rootCmd.MarkFlagRequired("config")

	//rootCmd.AddCommand(evm_transfer_history_task.EvmContractCmd)
	rootCmd.AddCommand(nftcmd.Command())
	// 离线回放指定高度区间的 EVM 转账派生数据（Dry 模式：只写派生表，不写指针/台账）
	rootCmd.AddCommand(evmtransfercmd.Command())
	// 离线回放指定高度区间的 ERC20 派生数据（同上，另需 ABI 解码器与 lotus 节点）
	rootCmd.AddCommand(erc20cmd.Command())
	// 离线回放指定高度区间/高度清单的 FNS 派生数据（task + calculator，可 --skip-calculator）
	rootCmd.AddCommand(fnscmd.Command())
	// 离线回放指定高度区间/高度清单的变动 Actor 派生数据（只跑计算器，补 chain.actor_actions）
	rootCmd.AddCommand(actoractionscmd.Command())

	// === 矿工/所有者统计链的离线回放（**按下面的顺序跑**，即依赖图的拓扑序）===
	// 说明：chain.miner_infos / chain.owner_infos / chain.abs_power_change 由既有的 chain-miner 负责；
	//       chain.miner_stats / chain.owner_stats / chain.sync_miner_epochs 由 calc-miner-owner-task
	//       的离线命令负责（另一个并行分支在做，不在本分支）。
	// 1. 补 chain.builtin_actor_states（最底层输入：输入只有节点侧 actor 状态，无数据库依赖）
	rootCmd.AddCommand(baselineactorscmd.Command())
	// 2. 补 chain.miner_rewards / chain.owner_rewards / chain.miner_win_counts（sync task reward-task）
	rootCmd.AddCommand(minerrewardscmd.Command())
	// 3. 补 chain.miner_gas_fees / chain.method_gas_fees / chain.base_gas_costs（sync task trace-task，读 traces）
	rootCmd.AddCommand(minergasfeecmd.Command())
	// 3b. 补 chain.base_gas_costs 的 sector_fee32 / sector_fee64 两列（计算器，依赖 3 写的 base_gas_costs）
	rootCmd.AddCommand(minergasestimatecmd.Command())
	// 4. 补 chain.miner_reward_stats（计算器，依赖 2 的 miner_rewards 与 1 的 builtin_actor_states）
	rootCmd.AddCommand(mineraccrewardcmd.Command())
	// 5. 刷新 chain.miner_agg_rewards（计算器，依赖 2 的 miner_rewards / miner_win_counts；只支持区间模式）
	rootCmd.AddCommand(mineraggrewardcmd.Command())

	rootCmd.AddCommand(procmd.ProInfoCommand())
	rootCmd.AddCommand(procmd.ProRewardCommand())
	rootCmd.AddCommand(procmd.ProSectorCommand())
	rootCmd.AddCommand(minertaskcmd.Command())

	//rootCmd.AddCommand(evm_transfer_history_task.EvmContractCmd)
	// ./filscan-syncer evm-actor -c ./config.toml --start --ebd
}

func newApp(conf *config.Config, adapter londobell.Adapter, sy *syncer.Manager,
	ml *ipquery.MinerLocationTask, fa *fullactors.Syncer, api syncer_api.AdminAPI,
	filPriceSyncer *price_syncer.FilpriceTask) *gin.Engine {

	var err error
	if conf.TestNet {
		var r *londobell.EpochReply
		r, err = adapter.Epoch(context.Background(), nil)
		if err != nil {
			panic(fmt.Errorf("get adapter epoch error: %s", err))
		}
		chain.RegisterBaseTime(r.Epoch, r.BlockTime)
	}
	chain.RegisterNet(conf.TestNet)

	// 同步补全 actor 创建时间
	if conf.UpdateCreateTime {
		go fa.Sync()
	}

	if conf.SyncerTask {
		err = sy.Run()
		if err != nil {
			panic(err)
		}
	}

	if conf.IpTask {
		go ml.Run()
	}

	go filPriceSyncer.Run()

	server := mix.NewServer()
	server.RegisterAPI(server.Group("/admin"), "v0", api)

	return server.Engine
}

func runCmd(_ *cobra.Command, _ []string) {

	// 读取配置文件
	conf := &config.Config{}
	err := _config.UnmarshalConfigFile(configFile, conf)
	if err != nil {
		log.Fatal(err)
	}
	spew.Json(conf)

	app, cleanup, err := wireApp(conf)
	if err != nil {
		panic(err)
	}
	defer cleanup()

	go func() {
		// start and wait for stop signal
		if e := app.Run(*conf.SyncerAddress); e != nil {
			panic(e)
		}
	}()

	_app.AddExitHandler(func() error {
		cleanup()
		return nil
	})
	_app.WaitExit()
}

func main() {
	_ = rootCmd.Execute()
}
