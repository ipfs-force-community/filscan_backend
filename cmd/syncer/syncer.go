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
