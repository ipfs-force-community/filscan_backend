package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/rpc"
	"github.com/gozelle/gin"
	"github.com/gozelle/mix"
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/decoder"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// go build -ldflags "-X main.Version=x.y.z"
var (
	// Name is the name of the compiled software.
	Name string = "abi-decoder-local"
	// Version is the version of the compiled software.
	Version string

	id, _ = os.Hostname()
)

var (
	rootCmd    *cobra.Command
	configFile string
)

func init() {
	rootCmd = &cobra.Command{
		Use:     "abi-decoder [-c|--config /path/to/config.toml]",
		Short:   Name,
		Run:     runCmd,
		Args:    cobra.ExactArgs(0),
		Version: Version,
	}

	rootCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径")
	rootCmd.Flags().SortFlags = false
	_ = rootCmd.MarkFlagRequired("config")
}

func newApp(conf *config.Config, apiDecoder fevm.ABIDecoderAPI) *gin.Engine {

	server := mix.NewServer()
	server.RegisterRPC(server.Group("/api"), "abi", apiDecoder)

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

	db, cancel, err := NewGormDB(conf)
	if err != nil {
		panic(err)
	}

	//服务器地址
	//conn, err := ethclient.Dial("https://api.node.glif.io/rpc/v1")
	//conn, err := ethclient.Dial("https://filfox.info/rpc/v1")
	var conn *ethclient.Client
	if conf.ABINodeToken == nil {
		conn, err = ethclient.Dial(*conf.ABINode)
	} else {
		client, err := rpc.DialOptions(context.TODO(), *conf.ABINode, rpc.WithHeader("Authorization", "Bearer "+*conf.ABINodeToken))
		if err != nil {
			fmt.Println("DialOptions err", err)
			return
		}
		conn = ethclient.NewClient(client)
	}
	if err != nil {
		fmt.Println("Dial err", err)
		return
	}

	_app.AddExitHandler(func() error {
		conn.Close()
		return nil
	})

	app := newApp(conf, decoder.NewDecoder(db, conn))
	_app.AddExitHandler(func() error {
		cancel()
		return nil
	})

	go func() {
		err = app.Run(*conf.ABIDecoderAddress)
		if err != nil {
			panic(err)
		}
	}()

	_app.WaitExit()
}

func main() {
	_ = rootCmd.Execute()
}

func NewGormDB(conf *config.Config) (*gorm.DB, func(), error) {

	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold:             5 * time.Second, // Slow SQL threshold
			LogLevel:                  logger.Error,    // Log level
			IgnoreRecordNotFoundError: true,            // Ignore ErrRecordNotFound error for logger
			Colorful:                  true,            // Disable color
		},
	)

	db, err := newGormDB(*conf.DSN, &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		return nil, nil, err
	}
	_db, err := db.DB()
	if err != nil {
		return nil, nil, err
	}

	_db.SetMaxOpenConns(200)
	_db.SetConnMaxIdleTime(60 * time.Second)

	return db, func() {
		d, _ := db.DB()
		if d != nil {
			_ = d.Close()
		}
	}, nil
}

func newGormDB(dsn string, opts ...gorm.Option) (*gorm.DB, error) {
	_opts := make([]gorm.Option, 0)
	if len(opts) == 0 {
		_opts = append(_opts, &gorm.Config{
			Logger: logger.Default,
		})
	} else {
		_opts = append(_opts, opts...)
	}
	db, err := gorm.Open(postgres.Open(dsn), _opts...)
	if err != nil {
		err = fmt.Errorf("connect postgres error: %s", err)
		return nil, err
	}
	_db, err := db.DB()
	if err != nil {
		err = fmt.Errorf("get postgres sql.DB error: %s", err)
		return nil, err
	}
	err = _db.Ping()
	if err != nil {
		err = fmt.Errorf("ping postgres error:%s", err)
		return nil, err
	}
	return db, nil
}
