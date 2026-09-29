// Package actoractionscmd 提供「离线回放指定高度区间 / 指定高度清单的变动 Actor 派生数据」子命令。
//
// 用途（主网缺口回补）：把某段高度区间或某份高度清单的 chain.actor_actions / chain.actors
// 派生数据补回来，而**完全不碰**同步指针与台账（chain.sync_syncers、chain.sync_task_epochs、
// chain.sync_syncer_epochs、chain.sync_skipped_epochs 一个都不写）。
//
// 为什么需要它（背景）：计算器在「该高度的 actor 余额(chain.actor_balances)还没落库」时
// 以 `return nil` 静默空跑，框架据此把该高度记为「已执行」（chain.sync_task_epochs.cost = 0），
// 后续每轮都在「该高度是否已执行」处直接跳过 ⇒ chain.actor_actions 静默漏数
// （实测 [6280000, 6412000] 内漏 909 段 / 1333 个高度）。该缺陷已在 bb20274 修掉
// （现在返回可重试的软错误、不再记为已执行），但**历史漏掉的高度不会自愈**。
//
// 为什么不能「把 chain.sync_syncers.epoch 回拨到缺口起点重跑」：那会把整段（十几万高度）的
// 任务与计算器一起重算，代价不可接受；而这些缺口高度是**离散**的 —— 所以正解是按高度清单补
// （--epochs-file，见 modules/syncer/fevm/offline-replay 框架）。
//
// 与生产 actor 同步器的关系（只跑计算器，不跑 task）：
//   - 生产 actor 同步器 = task(change-actor-task → chain.actor_balances) +
//     calculator(calc-change-actor-task → chain.actor_actions / chain.actors)；
//   - 本命令**只注册计算器**：chain.actor_balances 已经就位，跑 task 只会重写余额、白费
//     adapter 调用（ChangeActors / LastEpoch）；
//   - 因此与生产一致地不注册 SetTracesBuilder（Target.SkipTraces = true）：计算器不读 traces。
//
// 幂等性提醒：chain.actor_actions 的主键是 (epoch, actor_id)，同一高度跑第二遍会撞主键
// （写库报错、该高度重试后被保险丝放弃并列进报告）。要重跑先按高度删旧行：
//
//	delete from chain.actor_actions where epoch = <高度>;
//
// （chain.actors 是先按 id 删再插，重复回放不会累积重复行；chain.actor_balances 本命令不写。）
package actoractionscmd

import (
	"log"

	"github.com/spf13/cobra"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	filscansyncer "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_change_actor_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-change-actor-task"
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

// Command 离线回放变动 Actor 派生数据（指定高度区间或高度清单，只写派生表）
func Command() *cobra.Command {
	option := options{}
	cmd := &cobra.Command{
		Use: "actor-actions [-c|--config /path/to/config.toml] (--start <高度> --end <高度> | " +
			"--epochs-file <清单文件>) [--no-write]",
		Short: "离线回放指定高度区间/高度清单的变动 Actor 派生数据（补 chain.actor_actions）",
		Long: "离线回放高度区间 [--start, --end]（左闭右闭）或高度清单 [--epochs-file]（一行一个高度，\n" +
			"升序逐个高度）的变动 Actor 派生数据，补 chain.actor_actions / chain.actors\n" +
			"（只跑计算器 calc-change-actor-task；chain.actor_balances 已就位，不重跑 change-actor-task）。\n" +
			"两者互斥：离散缺口（如历史漏掉的 1333 个高度）必须用清单 —— 按区间重跑会把整段一起重算，\n" +
			"代价不可接受；清单模式下一个高度一个同步器，某高度失败只重跑该高度，\n" +
			"同一高度连续失败达上限（默认 3 次）即放弃该高度并继续后续高度，不阻断整批。\n\n" +
			"固定以 syncer 的 Dry 模式运行：只执行计算器（派生表写入），不写 chain.sync_syncers 进度指针、\n" +
			"不写 chain.sync_task_epochs / chain.sync_syncer_epochs、不写 chain.sync_skipped_epochs 跳过台账，\n" +
			"也不做链一致性检查与回滚。\n\n" +
			"注意（两个语义边界，跑之前先对齐业务）：\n" +
			"  1. 计算器用 adapter 取 actor 信息，取到的是**回溯高度当时**的状态；\n" +
			"  2. `chain.actor_actions.action` 的 新增(1)/更新(2) 依据是 chain.actors 里是否已有该 id，\n" +
			"     补历史高度时该判据反映的是**当前库**的状态，不是该高度当时的状态。\n\n" +
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

	target, writes := buildTarget(deps.DB, option.noWrite)

	opt := offlinereplay.RunOptions{NoWrite: option.noWrite}
	opt.EpochsChunk, opt.EpochsThreshold = deps.EpochsConfig()

	return offlinereplay.Execute(plan, opt, target, deps.DB, deps.Agg, deps.Adapter, writes)
}

// resolvePlan 计划解析的唯一入口（单独抽出来，单测可直接覆盖参数校验：Run 里非法参数会 log.Fatal）
func resolvePlan(option options) (offlinereplay.Plan, error) {
	return offlinereplay.ResolvePlan(option.start, option.end, option.epochsFile)
}

// newChangeActorRepo 构造变动 Actor 仓储。抽成包级变量只是为了让单测能注入假仓储
// （计算器要读 chain.actor_balances / chain.actors，离线的假连接无法应答那些 SQL），
// 生产路径始终是 dal.NewChangeActorTaskDal。
var newChangeActorRepo = func(db *gorm.DB) repository.ChangeActorTask { return dal.NewChangeActorTaskDal(db) }

// buildTarget 装配回放目标：同步器名与生产一致（actor），只注册计算器 CalcChangeActorTask。
// --no-write 时用「只统计不落库」包装包住仓储，并返回写统计来源。
func buildTarget(db *gorm.DB, noWrite bool) (offlinereplay.Target, func() []offlinereplay.WriteStat) {
	var repo repository.ChangeActorTask = newChangeActorRepo(db)

	target := offlinereplay.Target{
		Name: filscansyncer.ActorSyncer,
		// 与生产 actor 同步器一致：没有 WithContextBuilder（计算器不读 Datamap 里的 traces）。
		// 于是省掉每个高度一次 Traces 调用，也避免 traces 取不到时整个高度失败。
		SkipTraces: true,
	}

	if !noWrite {
		target.Calculators = []filscansyncer.Calculator{calc_change_actor_task.NewCalcChangeActorTask(repo)}
		return target, nil
	}

	wrapped := NewNoWriteChangeActorRepo(repo)
	target.Calculators = []filscansyncer.Calculator{calc_change_actor_task.NewCalcChangeActorTask(wrapped)}
	return target, wrapped.WriteStats
}
