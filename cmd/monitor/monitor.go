package main

import (
	"context"
	"fmt"
	"os"

	mbiz "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/service"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/global"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/biz/notify"

	"github.com/gozelle/gin"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/mix"
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string = "monitor"
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
		Use:     "filscan-monitor [-c|--config /path/to/config.toml]",
		Short:   Name,
		Run:     runCmd,
		Args:    cobra.ExactArgs(0),
		Version: Version,
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径")
	rootCmd.Flags().SortFlags = false
	_ = rootCmd.MarkFlagRequired("config")

	// ./filscan-syncer evm-actor -c ./config.toml --start --ebd
}

func newApp(conf *config.Config, adapter londobell.Adapter, notify *notify.Notify, g *global.GlobalBiz, rw *mbiz.WatcherBiz) *gin.Engine {

	var err error
	if conf.TestNet {
		chain.RegisterNet(conf.TestNet)
		var r *londobell.EpochReply
		r, err = adapter.Epoch(context.Background(), nil)
		if err != nil {
			panic(fmt.Errorf("get adapter epoch error: %s", err))
		}
		chain.RegisterBaseTime(r.Epoch, r.BlockTime)
	}
	ctx := context.Background()
	go rw.RuleWatch(ctx)
	go rw.VipWatch(ctx)
	server := mix.NewServer()
	//server.RegisterAPI(server.Group("/monitor"), "v0", api)

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
		if e := app.Run(*conf.MonitorAddress); e != nil {
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
