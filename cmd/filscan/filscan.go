package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/gozelle/gin"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/mix"
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/bearer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/vip"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"gorm.io/gorm"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string = "api-local"
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
		Use:     "filscan-api [-c|--config /path/to/config.toml]",
		Short:   Name,
		Run:     runCmd,
		Args:    cobra.ExactArgs(0),
		Version: Version,
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径")
	rootCmd.Flags().SortFlags = false
	_ = rootCmd.MarkFlagRequired("config")
}

func newApp(conf *config.Config, adapter londobell.Adapter, fullApi filscan.BrowserAPI, proApi pro.FullAPI, db *gorm.DB, redis *redis.Redis) *gin.Engine {

	chain.RegisterNet(conf.TestNet)
	r, err := adapter.Epoch(context.Background(), nil)
	if err != nil {
		panic(fmt.Errorf("get adapter epoch error: %s", err))
	}
	chain.RegisterBaseTime(r.Epoch, r.BlockTime)

	server := mix.NewServer()
	server.RegisterAPI(server.Group("/api", func(c *gin.Context) {
		ctx := c.Request.Context()
		ctx = context.WithValue(ctx, "url", c.Request.URL.String()) //nolint
		c.Request = c.Request.WithContext(ctx)
	}), "v1", fullApi)

	server.RegisterAPI(server.Group("/pro", bearer.Authentication(), wrapCustomError(), vip.AuthenticationWithVIP(db, redis)), "v1", proApi)

	//server.RegisterAPI(server.Group("/api/cron"), "v1", cronApi)
	return server.Engine
}

type bodyLogWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *bodyLogWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func wrapCustomError() gin.HandlerFunc {
	return func(c *gin.Context) {
		buff := &bytes.Buffer{}
		originalWriter := c.Writer
		c.Writer = &bodyLogWriter{
			ResponseWriter: originalWriter,
			body:           buff,
		}
		c.Next()
		if c.Writer.Status() == 400 {
			originalWriter.WriteHeader(200)
		}
		_, _ = originalWriter.WriteString(buff.String())
	}
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
		if e := app.Run(*conf.APIAddress); e != nil {
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
