package procmd

import (
	"github.com/gozelle/spew"
	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_miner_acc_reward_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-acc-reward-task"
	calc_miner_agg_reward "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-agg-reward"
	builtin_actor_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/builtin-actor-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/chain/reward_task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm_transaction/evm_cmd"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_app"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"log"
)

type rewardOptions struct {
	start   int64
	end     int64
	config  string
	reward  bool
	refresh bool
}

func ProRewardCommand() *cobra.Command {
	option := rewardOptions{}
	cmd := &cobra.Command{
		Use:   "pro-reward [-c|--config /path/to/config.toml]",
		Short: "pro-reward",
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
			
			var tasks []syncer.Task
			var calculators []syncer.Calculator
			
			if option.reward {
				tasks = append(tasks,
					builtin_actor_task.NewBaselineTask(dal.NewBaseLineTaskDal(db)), // 同步内置 Actor 状态, 爆块奖励依赖此任务
					reward_task.NewMinerRewardTask(dal.NewRewardTaskDal(db)),       // 记录 Miner爆块奖励 及 WinCount
				)
				calculators = append(calculators,
					calc_miner_acc_reward_task.NewCalcMinerAccRewardTask(dal.NewRewardTaskDal(db)), // 计算 Miner 奖励统计值，依赖历史高度汇总值
				)
			}
			
			if option.refresh {
				calculators = append(calculators,
					calc_miner_agg_reward.NewCalcMinerAggReward(dal.NewRewardTaskDal(db)), // 计算 Miner 的历史统计值，提供 Pro 使用
				)
			}
			
			s := syncer.NewSyncer(
				syncer.WithName(syncer.ChainSyncer),
				syncer.WithDB(db),
				syncer.WithLondobellAgg(agg),
				syncer.WithLondobellMinerAgg(minerAgg),
				syncer.WithLondobellAdapter(adapter),
				syncer.WithInitEpoch(&option.start),
				syncer.WithStopEpoch(&option.end),
				syncer.WithEpochsChunk(*conf.Syncer.EpochsChunk),
				syncer.WithEpochsThreshold(*conf.Syncer.EpochsThreshold),
				syncer.WithDry(true),
				syncer.WithContextBuilder(injector.SetTracesBuilder),
				syncer.WithTaskGroup(tasks),
				syncer.WithCalculators(calculators...),
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
	cmd.Flags().BoolVar(&option.reward, "reward", false, "同步奖励")
	cmd.Flags().BoolVar(&option.refresh, "refresh", false, "刷新奖励")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")
	
	cmd.AddCommand(evm_cmd.EvmContractCmd)
	
	return cmd
}
