package evm_cmd

import (
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	c "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm_transaction"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"log"
	"strconv"
)

var EvmContractCmd *cobra.Command

var (
	configFile, startEpoch, endEpoch string
	//chunk, threshold string
)

// 2683348, 2849393,
// 2861403
// 3123055
func init() {

	EvmContractCmd = &cobra.Command{
		Use: "evm-transaction [-c|--config /path/to/config.toml]",
		Run: func(cmd *cobra.Command, args []string) {
			conf := &c.Config{}
			err := _config.UnmarshalConfigFile(configFile, conf)
			if err != nil {
				log.Fatal(err)
			}
			spew.Json(conf)

			db, _, err := injector.NewGormDB(conf)
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

			start, err := strconv.ParseInt(startEpoch, 10, 64)
			if err != nil {
				return
			}
			end, err := strconv.ParseInt(endEpoch, 10, 64)
			if err != nil {
				return
			}
			//ch, err := strconv.ParseInt(chunk, 10, 64)
			//if err != nil {
			//	return
			//}
			//th, err := strconv.ParseInt(threshold, 10, 64)
			//if err != nil {
			//	return
			//}

			s := syncer.NewSyncer(
				syncer.WithContextBuilder(injector.SetTracesBuilder),
				//syncer.WithIgnoreCheckEpoch(chain.Epoch(end)),
				syncer.WithName("evm-transaction-syncer"),
				syncer.WithDB(db),
				syncer.WithLondobellAgg(agg),
				syncer.WithLondobellAdapter(adapter),
				syncer.WithInitEpoch(&start),
				syncer.WithStopEpoch(&end),
				syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
				syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
				syncer.WithDry(true),
				syncer.WithTaskGroup(
					[]syncer.Task{
						evm_transaction.NewEvmTransactionTask(dal.NewEvmTransactionDal(db)), // 同步 EVM 类型 (FEVM合约) 地址
					},
				),
				syncer.WithCalculators(
					evm_transaction.NewEvmContractCalculator(dal.NewEvmTransactionDal(db)),
				),
			)
			err = s.Init()
			if err != nil {
				return
			}
			go s.Run()
			_app.WaitExit()
		},
		Args: cobra.ExactArgs(0),
	}

	EvmContractCmd.Flags().StringVarP(&configFile, "config", "c", "", "配置文件路径")
	EvmContractCmd.Flags().StringVarP(&startEpoch, "start", "s", "", "起始高度")
	EvmContractCmd.Flags().StringVarP(&endEpoch, "end", "e", "", "终止高度")
	//EvmContractCmd.Flags().StringVarP(&chunk, "chunk", "k", "", "最大并发数")
	//EvmContractCmd.Flags().StringVarP(&threshold, "threshold", "t", "", "步长阈值")
	EvmContractCmd.Flags().SortFlags = false
	_ = EvmContractCmd.MarkFlagRequired("config")

}
