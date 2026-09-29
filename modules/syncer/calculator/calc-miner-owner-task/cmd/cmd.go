// Package minerownercmd 提供「离线回放指定高度区间 / 指定高度清单的 Miner/Owner 统计派生数据」子命令。
//
// 用途（主网缺口回补）：把某段高度区间或某份高度清单的
// chain.sync_miner_epochs（计算器台账）+ chain.miner_stats + chain.owner_stats 补回来，
// 而**完全不碰**同步指针与同步器的任务台账（chain.sync_syncers、chain.sync_task_epochs、
// chain.sync_syncer_epochs、chain.sync_skipped_epochs 一个都不写）。
//
// 为什么需要它（业务背景）：
//   - calc-miner-owner-task 只在 ctx.Epoch()%120 == 0 的整点高度干重活（见
//     calc_miner_owner_task.go:85 的非测试网分支），非整点高度直接 return nil；
//   - 页面「这个整点算完了吗」的判据就是 chain.sync_miner_epochs 里有没有该高度的行
//     （计算器 save() 的第一句写入，见 calc_miner_owner_task.go:460）。这一行缺失 =
//     该整点的 Miner/Owner 统计在页面上等于「没算」；
//   - 该计算器的输入**全部来自数据库表**（win counts / rewards / gas fees / miner_infos，
//     走 repo 查询），不读 traces —— 于是可以完全离线按高度补算。
//
// 与生产 Miner 同步器的关系（只跑计算器，不跑任务）：
//   - 生产 Miner 同步器 = task(miner-task → chain.miner_infos / owner_infos / abs_power_change)
//     + calculator(calc-miner-owner-task → 本命令补的这三张表)；
//   - 本命令**只注册计算器 calc-miner-owner-task**：chain.miner_infos 已经就位，
//     跑 miner-task 只会重写 MinerInfos、白费 adapter 调用；
//   - 因此与生产一致地不注册 SetTracesBuilder（Target.SkipTraces = true）：本计算器不读 traces
//     （全包内没有任何 Trace/Datamap 引用，输入只有 repo 查询 + 一次 PowerActor 状态查询）。
//
// 前提条件（跑之前请确认）：链同步器已越过要补的高度。计算器会先读
// chain.sync_syncers 里 chain 同步器的 epoch（calc_miner_owner_task.go:92），
// 未到就打日志 + 等 15s 再查（第 99~101 行）—— 回补历史高度时该行早已远高于目标高度，
// 但若库里的 chain 行被卡住，本命令会在该高度上一直空等而不报错。
//
// 幂等性提醒（按高度重跑前先删旧行）：
//   - chain.sync_miner_epochs 有唯一索引 sync_miner_epochs_epoch_uindex(epoch)
//     （migration/1.chain.sql:302），同一高度跑第二遍会撞唯一键报错
//     （清单模式下该高度重试到上限后被放弃并列进报告）；
//   - chain.miner_stats / chain.owner_stats 只有普通索引，重跑会写出重复行（不会报错）。
//
// 所以按高度重跑的正确姿势是先删这三张表里该高度的行：
//
//	delete from chain.sync_miner_epochs where epoch = <高度>;
//	delete from chain.miner_stats      where epoch = <高度>;
//	delete from chain.owner_stats      where epoch = <高度>;
package minerownercmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_miner_owner_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task/luck"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gorm.io/gorm"
)

type options struct {
	config     string
	start      int64
	end        int64
	epochsFile string
	noWrite    bool
}

// Command 离线回放 Miner/Owner 统计派生数据（指定高度区间或高度清单，只跑计算器）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "calc-miner-owner [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的 Miner/Owner 统计（补 chain.sync_miner_epochs 台账）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的 Miner/Owner 统计派生数据，补 chain.sync_miner_epochs（台账）/\n" +
			"chain.miner_stats / chain.owner_stats（只跑计算器 calc-miner-owner-task；\n" +
			"chain.miner_infos 已就位，不重跑 miner-task）。\n" +
			"两者互斥：离散缺口必须用清单 —— 按区间重跑会把整段一起重算，代价不可接受；\n" +
			"清单模式下一个高度一个同步器，某高度失败只重跑该高度，\n" +
			"同一高度连续失败达上限（默认 3 次）即放弃该高度并继续后续高度，不阻断整批。\n\n" +
			"写什么：只有「整点高度」（高度 %120 == 0，非测试网）会真正写入；非整点高度计算器直接空跑\n" +
			"（calc_miner_owner_task.go:85）。每个整点高度写 1 行 chain.sync_miner_epochs\n" +
			"（每行 = 该高度已算完的判据，页面据此判断整点是否算完）+ 若干行 chain.miner_stats /\n" +
			"chain.owner_stats（24h/7d/30d 三个区间各一批；高度 %2880 == 2160 时另加日结与 1y 批次）。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行计算器（上述三张派生表的写入），\n" +
			"不写 chain.sync_syncers 进度指针、不写 chain.sync_task_epochs / chain.sync_syncer_epochs、\n" +
			"不写 chain.sync_skipped_epochs 跳过台账，也不做链一致性检查与回滚。\n\n" +
			"前提条件：chain.sync_syncers 里 chain 同步器的 epoch 必须已越过要补的高度，\n" +
			"否则计算器会每 15s 一轮空等（不报错、不继续）。\n\n" +
			"幂等性提醒：chain.sync_miner_epochs 有唯一索引 (epoch)，同一高度跑第二遍会撞唯一键报错；\n" +
			"chain.miner_stats / chain.owner_stats 重跑会写出重复行。重跑前请先删该高度的旧行：\n" +
			"  delete from chain.sync_miner_epochs where epoch = <高度>;\n" +
			"  delete from chain.miner_stats / chain.owner_stats where epoch = <高度>;\n\n" +
			"--no-write：跑完整条管线但不写任何派生表，只输出统计（处理高度数 / 聚合器调用次数 / 耗时 /\n" +
			"预计写入行数），用于安全测量；该模式下不会产生任何数据变更。",
		Run: func(_ *cobra.Command, _ []string) {
			if err := run(option); err != nil {
				log.Fatal(err)
			}
		},
	}

	cmd.Flags().StringVarP(&option.config, "config", "c", "", "配置文件路径")
	cmd.Flags().Int64VarP(&option.start, "start", "s", 0, "起始高度（含）")
	cmd.Flags().Int64VarP(&option.end, "end", "e", 0, "截止高度（含）")
	cmd.Flags().StringVar(&option.epochsFile, "epochs-file", "",
		"高度清单文件（一行一个高度，可含空行与 # 注释；与 --start/--end 互斥；内部升序去重后逐个高度回放）")
	cmd.Flags().BoolVar(&option.noWrite, "no-write", false,
		"只统计不落库：跑完整条管线但不写任何派生表，只输出统计")
	cmd.Flags().SortFlags = false
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

// run 执行一次离线回放。抽成独立函数（而不是全写在 Run 里）是为了让单测能直接断言
// 「参数非法时的报错」，而不必穿过 log.Fatal（那会 os.Exit，测不了）。
// 返回值：计划/依赖装配失败才返回错误；单个高度失败不返回错误（记进报告，见框架注释）。
func run(option options) error {
	// 计划解析与校验放在最前面：参数不合法就不去连任何生产依赖
	plan, err := resolvePlan(option)
	if err != nil {
		return err
	}

	deps, err := offlinereplay.Connect(option.config)
	if err != nil {
		return err
	}
	defer deps.Close()
	log.Printf("离线回放计划: %s; %s", plan, offlinereplay.ConfLine(deps.Conf))

	target, writes := buildTarget(deps.Conf, deps.DB, option.noWrite)

	opt := offlinereplay.RunOptions{NoWrite: option.noWrite}
	opt.EpochsChunk, opt.EpochsThreshold = deps.EpochsConfig()

	return offlinereplay.Execute(plan, opt, target, deps.DB, deps.Agg, deps.Adapter, writes)
}

// resolvePlan 计划解析的唯一入口（单独抽出来，单测可直接覆盖参数校验：Run 里非法参数会 log.Fatal）
func resolvePlan(option options) (offlinereplay.Plan, error) {
	return offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
}

// minerDeps 是计算器需要的三组依赖（仓储 / 同步器查询器 / 幸运值计算器）。
// 抽成包级构造器只是为了让单测能整体换成假实现（计算器的输入全部是库表查询，
// 离线的假连接无法应答那些 SQL），生产路径始终是 dal 的三个构造器。
type minerDeps struct {
	repo repository.MinerTask
	sg   repository.SyncerGetter
	luck *luck.Calculator
}

// newMinerDeps 生产依赖构造（dal 仓储 / dal 同步器查询器 / dal 幸运值仓储）
var newMinerDeps = func(db *gorm.DB) minerDeps {
	return minerDeps{
		repo: dal.NewMinerTaskDal(db),
		sg:   dal.NewSyncerDal(db),
		luck: luck.NewCalculator(dal.NewLuckDal(db)),
	}
}

// buildTarget 装配回放目标：同步器名与生产一致（miner），只注册计算器 calc-miner-owner-task。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(conf *config.Config, db *gorm.DB, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	deps := newMinerDeps(db)

	target := offlinereplay.Target{
		Name: filscansyncer.MinerSyncer,
		// 本计算器不读 traces（输入全是库表查询 + 一次 PowerActor 状态查询），与生产
		// Miner 同步器的 task 侧无关 —— 本命令也不注册 miner-task。
		// 于是省掉每个高度一次 Traces 调用，也避免 traces 取不到时整个高度失败。
		SkipTraces: true,
	}

	if !noWrite {
		target.Calculators = []filscansyncer.Calculator{
			calc_miner_owner_task.NewCalcMinerOwnerTaskWithDeps(conf, deps.repo, deps.sg, deps.luck),
		}
		return target, nil
	}

	wrapped := NewNoWriteMinerTaskRepo(deps.repo)
	target.Calculators = []filscansyncer.Calculator{
		calc_miner_owner_task.NewCalcMinerOwnerTaskWithDeps(conf, wrapped, deps.sg, deps.luck),
	}
	return target, wrapped.WriteStats
}
