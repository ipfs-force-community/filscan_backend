package procmd

import (
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	prosyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm_transaction/evm_cmd"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"log"
)

type proOptions struct {
	start     int64
	end       int64
	config    string
	noInfo    bool
	noBalance bool
	noFund    bool
}

func ProInfoCommand() *cobra.Command {
	option := proOptions{}
	cmd := &cobra.Command{
		Use:   "pro-info [-c|--config /path/to/config.toml]",
		Short: "pro-info",
		Run: func(cmd *cobra.Command, args []string) {
			
			var err error
			defer func() {
				if err != nil {
					log.Fatal(err)
				}
			}()
			
			conf := &config.Config{}
			err = _config.UnmarshalConfigFile(option.config, conf)
			if err != nil {
				return
			}
			spew.Json(conf)
			
			db, cancel, err := injector.NewGormDB(conf)
			if err != nil {
				return
			}
			defer func() {
				cancel()
			}()
			
			agg, err := injector.NewLondobellAgg(conf)
			if err != nil {
				return
			}
			
			adapter, err := injector.NewLondobellAdapter(conf)
			if err != nil {
				return
			}
			
			minerAgg, err := injector.NewLondobellMinerAgg(conf)
			if err != nil {
				return
			}
			
			s := syncer.NewSyncer(
				syncer.WithName(syncer.ProSyncer),
				syncer.WithDB(db),
				syncer.WithLondobellAgg(agg),
				syncer.WithLondobellMinerAgg(minerAgg),
				syncer.WithLondobellAdapter(adapter),
				syncer.WithInitEpoch(&option.start),
				syncer.WithStopEpoch(&option.end),
				syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
				syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
				syncer.WithDry(true),
				syncer.WithContextBuilder(injector.ProChecker),
				syncer.WithCalculators(
					prosyncer.NewProCalculator(db, prosyncer.ProCalculatorParams{
						Store:              true,
						DisableSyncInfo:    option.noInfo,
						DisableSyncFund:    option.noFund,
						DisableSyncBalance: option.noBalance,
					}), // 汇总 Pro 需要的 Miner 数据
				),
			)
			err = s.Init()
			if err != nil {
				return
			}
			go s.Run()
			
			_app.WaitExit()
		},
	}
	
	cmd.Flags().StringVar(&option.config, "config", "", "配置文件路径")
	cmd.Flags().Int64Var(&option.start, "start", 0, "开始高度，大于等于")
	cmd.Flags().Int64Var(&option.end, "end", 0, "截止高度，小于等于")
	cmd.Flags().BoolVar(&option.noInfo, "no-info", false, "不同步 Info")
	cmd.Flags().BoolVar(&option.noBalance, "no-balance", false, "不包括同步余额")
	cmd.Flags().BoolVar(&option.noFund, "no-fund", false, "不同步费用")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	
	cmd.AddCommand(evm_cmd.EvmContractCmd)
	
	return cmd
}
