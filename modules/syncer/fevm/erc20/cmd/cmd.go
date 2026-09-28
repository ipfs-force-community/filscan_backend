// Package erc20cmd 提供「离线回放指定高度区间的 ERC20 派生数据」子命令。
//
// 用途（主网缺口回补）：把某段高度区间的 ERC20 派生数据补回来，而**完全不碰**同步指针与台账
// （chain.sync_syncers / chain.sync_task_epochs / chain.sync_syncer_epochs / chain.sync_skipped_epochs 都不写）。
//
// 与 evm-transfer 的差别（回放前请先读这段）：
//   - 除 traces 外还依赖 ABI 解码器与 lotus 节点（生产 Erc20Syncer 亦然）：每条转账都要读代币
//     名称/精度/余额，因此同样高度下的外部调用量比 evm-transfer 大得多（余额是逐条查节点）；
//   - fevm.erc20_balance 是**覆盖式** upsert（ON CONFLICT (owner, contract_id) DO UPDATE
//     SET amount = EXCLUDED.amount）：重复回放不会累加出错，但会把「当时余额」改写成「回放时刻的
//     链上余额」；历史区间的余额口径因此是「回放当刻值」，不是「该高度当时值」；
//   - fevm.erc_20_transfers / fevm.erc20_swap_info 是追加式写入，其唯一索引（若存在）会让同一区间
//     第二次回放撞唯一键；
//   - "新代币" 补扫分支（handleFreshErc20Tokens → UpdateOneERC20Contract 改 fevm.erc20_contract）
//     只对「距链头 30 分钟内」的高度生效，回放历史缺口时不会进入该分支。
package erc20cmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	erc20 "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/erc20"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
	"gorm.io/gorm"
)

type options struct {
	config     string
	start      int64
	end        int64
	epochsFile string
	noWrite    bool
}

// Command 离线回放 ERC20 派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "erc20 [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的 ERC20 派生数据（不写同步指针/台账）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的 ERC20 派生数据，补\n" +
			"fevm.erc_20_transfers / fevm.erc20_balance / fevm.erc20_swap_info。\n" +
			"两者互斥：离散缺口用清单（某高度失败只重跑该高度、单个坏高度不阻断整批），连续缺口用区间。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行任务（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"注意：本任务额外依赖 ABI 解码器与 lotus 节点（逐条读代币信息与余额），外部调用量显著大于\n" +
			"evm-transfer；且 fevm.erc20_balance 为覆盖式 upsert（回放会把余额改写为回放当刻的链上值）。\n\n" +
			"--no-write：跑完整条管线但不写任何派生表，只输出统计（处理高度数 / 聚合器调用次数 / 耗时 /\n" +
			"预计写入行数），用于安全测量；该模式下不会产生任何数据变更。",
		Run: func(cmd *cobra.Command, args []string) {

			var err error
			defer func() {
				if err != nil {
					log.Fatal(err)
				}
			}()

			// 计划解析与校验放在最前面：参数不合法就不去连任何生产依赖
			plan, err := offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
			if err != nil {
				return
			}

			deps, err := offlinereplay.Connect(option.config)
			if err != nil {
				return
			}
			defer deps.Close()
			log.Printf("离线回放计划: %s; %s", plan, offlinereplay.ConfLine(deps.Conf))

			abiDecoder, err := deps.AbiDecoder()
			if err != nil {
				return
			}
			node, err := deps.AbiNode()
			if err != nil {
				return
			}

			target, writes := buildTarget(deps.DB, abiDecoder, node, option.noWrite)

			opt := offlinereplay.RunOptions{NoWrite: option.noWrite}
			opt.EpochsChunk, opt.EpochsThreshold = deps.EpochsConfig()

			err = offlinereplay.Execute(plan, opt, target, deps.DB, deps.Agg, deps.Adapter, writes)
		},
	}

	cmd.Flags().StringVarP(&option.config, "config", "c", "", "配置文件路径")
	cmd.Flags().Int64VarP(&option.start, "start", "s", 0, "起始高度（含）")
	cmd.Flags().Int64VarP(&option.end, "end", "e", 0, "截止高度（含）")
	cmd.Flags().StringVar(&option.epochsFile, "epochs-file", "",
		"高度清单文件（一行一个高度，可含空行与 # 注释；与 --start/--end 互斥；内部升序去重后逐个高度回放）")
	cmd.Flags().BoolVar(&option.noWrite, "no-write", false,
		"只统计不落库：跑完整条 task 管线但不写任何派生表，只输出统计")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

// newERC20Repo 构造 ERC20 仓储。抽成包级变量只是为了让单测能注入假仓储
// （ERC20Task 的构造函数会读派生表里的方法签名表，离线的假连接无法应答该 SQL），
// 生产路径始终是 dal.NewERC20Dal。
var newERC20Repo = func(db *gorm.DB) repository.ERC20TokenRepo { return dal.NewERC20Dal(db) }

// buildTarget 装配回放目标：同步器名与生产一致（erc20），任务为 ERC20Task。
// 依赖与生产 Erc20Syncer 同源：ERC20 仓储 + ABI 解码器 + lotus 节点（此处只做注入，不拨号）。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, abiDecoder fevm.ABIDecoderAPI, node *lotus_api.Node,
	noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {

	var repo repository.ERC20TokenRepo = newERC20Repo(db)
	if !noWrite {
		return offlinereplay.Target{
			Name:   filscansyncer.Erc20Syncer,
			Groups: []filscansyncer.TaskGroup{{erc20.NewERC20Task(abiDecoder, repo, node)}},
		}, nil
	}

	wrapped := NewNoWriteERC20Repo(repo)
	return offlinereplay.Target{
		Name:   filscansyncer.Erc20Syncer,
		Groups: []filscansyncer.TaskGroup{{erc20.NewERC20Task(abiDecoder, wrapped, node)}},
	}, wrapped.WriteStats
}
