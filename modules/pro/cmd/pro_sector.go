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

type sectorOptions struct {
	start  int64
	end    int64
	config string
}

func ProSectorCommand() *cobra.Command {
	option := sectorOptions{}
	cmd := &cobra.Command{
		Use:   "pro-sector [-c|--config /path/to/config.toml]",
		Short: "pro-sector",
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
				syncer.WithName(syncer.SectorSyncer),
				syncer.WithDB(db),
				syncer.WithLondobellAgg(agg),
				syncer.WithLondobellMinerAgg(minerAgg),
				syncer.WithLondobellAdapter(adapter),
				syncer.WithInitEpoch(&option.start),
				syncer.WithStopEpoch(&option.end),
				syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
				syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
				syncer.WithDry(true),
				syncer.WithTaskGroup(syncer.TaskGroup{
					prosyncer.NewSectorTask(db, true),
				}),
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
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	
	cmd.AddCommand(evm_cmd.EvmContractCmd)
	
	return cmd
}
