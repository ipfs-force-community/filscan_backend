package minertaskcmd

import (
	"log"
	
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	miner_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/miner/miner-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
)

type options struct {
	start  int64
	end    int64
	config string
}

func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use:   "chain-miner [-c|--config /path/to/config.toml]",
		Short: "chain-miner",
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
			
			task := miner_task.NewMinerInfoTask(conf, dal.NewSyncEpochGetterDal(db), dal.NewMinerTaskDal(db))
			task.GapScan = true
			s := syncer.NewSyncer(
				syncer.WithName(syncer.MinerSyncer),
				syncer.WithDB(db),
				syncer.WithLondobellAgg(agg),
				syncer.WithLondobellAdapter(adapter),
				syncer.WithInitEpoch(&option.start),
				syncer.WithStopEpoch(&option.end),
				syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
				syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
				syncer.WithDry(true),
				syncer.WithContextBuilder(injector.StateChecker),
				syncer.WithTaskGroup(
					[]syncer.Task{
						task, // 记录 MinerInfos
					},
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
	
	cmd.Flags().StringVarP(&option.config, "config", "c", "", "配置文件路径")
	cmd.Flags().Int64VarP(&option.start, "start", "s", 0, "配置文件路径")
	cmd.Flags().Int64VarP(&option.end, "end", "e", 0, "配置文件路径")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	
	//cmd.AddCommand(evm_transfer_history_task.EvmContractCmd)
	
	return cmd
}
