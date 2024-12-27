package nftcmd

import (
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/nft"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"log"
)

type options struct {
	start  int64
	end    int64
	config string
}

func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use:   "nft [-c|--config /path/to/config.toml]",
		Short: "nft",
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
			
			decoder, err := injector.NewAbiDecoderClient(conf)
			if err != nil {
				return
			}
			
			agg, err := injector.NewLondobellAgg(conf)
			if err != nil {
				return
			}
			
			adapter, err := injector.NewLondobellAdapter(conf)
			if err != nil {
				return
			}
			
			s := syncer.NewSyncer(
				syncer.WithName(syncer.NFTSyncer),
				syncer.WithDB(db),
				syncer.WithLondobellAgg(agg),
				syncer.WithLondobellAdapter(adapter),
				syncer.WithInitEpoch(&option.start),
				syncer.WithStopEpoch(&option.end),
				syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
				syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
				syncer.WithDry(true),
				syncer.WithTaskGroup(
					[]syncer.Task{
						nft.NewNFTTask(db, decoder),
					},
				),
				syncer.WithCalculators(
					nft.NewNFTCalculator(db, decoder),
				),
				syncer.WithContextBuilder(injector.SetTracesBuilder),
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
