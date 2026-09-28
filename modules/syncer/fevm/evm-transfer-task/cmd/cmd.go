// Package evmtransfercmd 提供「离线回放指定高度区间的 EVM 转账派生数据」子命令。
//
// 用途（主网缺口回补）：把某段高度区间的 fevm.evm_transfers / fevm.evm_transfer_stats
// 派生数据补回来，而**完全不碰**同步指针与台账 —— 即 chain.sync_syncers、
// chain.sync_task_epochs、chain.sync_syncer_epochs、chain.sync_skipped_epochs 一个都不写。
//
// 为什么要走本命令，而不能靠「把 chain.sync_syncers.epoch 回拨到缺口起点重跑」：
// 所有 fevm 派生同步器都共用 injector.SetTracesBuilder，而非 Dry 模式下它会调用
// ctx.Adapter().Epoch(&epoch) 去校验该高度的 tipset；对历史缺口高度该调用必然失败
// （load state tree: failed to load hamt node —— 本地节点只保留近期状态窗口，历史状态已被裁掉），
// 于是回拨指针只会让同步器卡在缺口高度上反复重试，永远追不上。
//
// 两种模式：
//   - 默认（真写）：跑完区间并把派生数据写进 fevm.evm_transfers / fevm.evm_transfer_stats；
//   - --no-write：跑完整条 task 管线（含聚合器取数、actor 查询、聚合统计），
//     但派生表的写入与删除全部被拦下，只统计「调用次数 / 行数 / 聚合器调用次数 / 耗时」。
//     用于上线前安全测量压力与预计写入量，不产生任何数据变更。
//
// 加速开关 --skip-acc-stats（默认关闭 ⇒ 行为与不带该参数时完全一致）：
// 跳过每 120 高度的「1 小时累计快照」重算。该重算的 SQL 要扫 fevm.evm_transfers 全表
// （线上约 1049 万行）叠窗口函数排序，排序溢出 403MB 临时文件、实测单次 35 秒，是回放期间
// 最主要的纯耗时来源；而回放区间内这些「缺口边界」快照一行都不会被接口读到（接口只读 max(epoch)
// 的链头快照，由实时同步器维护），故离线回放可安全跳过。实时同步器不带该参数，行为不变。
//
// 幂等性提醒（重跑前必读）：fevm.evm_transfers 上有 message_cid 唯一索引
// （见 migration/12.evm_contract.sql），而本命令写的这张表仍是裸插入 —— 仓储侧
// （modules/common/infra/dal/dal_evm_transfer.go 的 SaveEvmTransfers）**没有**「先查后跳」守卫，
// 所以同一区间跑第二遍仍会撞唯一键（写库路径报错、该高度重试后仍失败）。
// 需要重跑时，先删除区间内的旧行
// （`delete from fevm.evm_transfers where epoch between <start> and <end>`）再跑。
// 别把这条与同目录的 erc20 / nft 回放混为一谈：那两条路径的派生表
// （fevm.erc_20_transfers / fevm.nft_transfers）已在应用层加了「先查后跳」守卫
// （dal 的 FilterExistingERC20Transfers、nft Mapper 的 cid 去重），重跑会自动跳过已写入的行，
// 不需要先删；本命令的 evm_transfers 目前还没有这层守卫。
package evmtransfercmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	evmtransfertask "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/evm-transfer-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

type options struct {
	config       string
	start        int64
	end          int64
	epochsFile   string
	noWrite      bool
	skipAccStats bool
}

// Command 离线回放 EVM 转账派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "evm-transfer [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write] [--skip-acc-stats]",
		Short: "离线回放指定高度区间/高度清单的 EVM 转账派生数据（不写同步指针/台账）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的 fevm EVM 转账派生数据，补 fevm.evm_transfers / fevm.evm_transfer_stats。\n" +
			"两者互斥：离散缺口（补几个高度）用清单，连续缺口用区间；清单模式下一个高度一个同步器，\n" +
			"某高度失败只重跑该高度，且单个坏高度不阻断整批。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行任务（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"--no-write：跑完整条管线但不写任何派生表，只输出统计（处理高度数 / 聚合器调用次数 / 耗时 /\n" +
			"预计写入行数），用于安全测量；该模式下不会产生任何数据变更。\n\n" +
			"--skip-acc-stats：跳过每 120 高度的 1 小时累计快照重算（离线回放专用）。该重算要扫\n" +
			"fevm.evm_transfers 全表叠窗口函数排序（线上实测单次约 35 秒），而回放区间内这些缺口边界\n" +
			"快照不被接口读取（接口只读 max(epoch) 的链头快照，由实时同步器维护），故离线回放可安全跳过。\n" +
			"默认关闭 ⇒ 不带该参数时行为与不带开关的历史版本完全一致。",
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

			target, writes := buildTarget(deps.DB, option.noWrite, option.skipAccStats)

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
	cmd.Flags().BoolVar(&option.skipAccStats, "skip-acc-stats", false,
		"跳过每 120 高度的 1 小时累计快照重算（离线回放专用；缺口边界快照不被接口读取）")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

// buildTarget 装配回放目标：同步器名与生产一致（evm-contract），任务为 EVMTransferTask。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
// skipAccStats 透传给任务（离线回放专用开关，默认 false ⇒ 行为与历史版本一致）。
func buildTarget(db *gorm.DB, noWrite, skipAccStats bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var repo repository.EvmTransferRepo = dal.NewEVMTransferDal(db)
	if !noWrite {
		return offlinereplay.Target{
			Name:   filscansyncer.EvmContractSyncer,
			Groups: []filscansyncer.TaskGroup{{evmtransfertask.NewEVMTransferTask(repo).WithSkipAccStats(skipAccStats)}},
		}, nil
	}

	wrapped := NewNoWriteEvmTransferRepo(repo)
	return offlinereplay.Target{
		Name:   filscansyncer.EvmContractSyncer,
		Groups: []filscansyncer.TaskGroup{{evmtransfertask.NewEVMTransferTask(wrapped).WithSkipAccStats(skipAccStats)}},
	}, wrapped.WriteStats
}
