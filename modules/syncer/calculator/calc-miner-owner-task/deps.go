package calc_miner_owner_task

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task/luck"
)

// 本文件只做一件事：把计算器的依赖装配从「db 写死成 dal 仓储」打开一个注入口。
//
// 为什么需要它（离线回放子命令 `calc-miner-owner` 的前提）：
//   - 单测要断言「哪些行会被写」，必须能把 repo / sg / luck 换成假实现；
//   - `--no-write` 要把全部写入拦下，必须能把 repo 换成「只统计不落库」的包装。
//
// 生产构造器 NewCalcMinerOwnerTask(conf, db) 把 db 直接固化成 dal 仓储，上面两件事都做不到
// （CalcMinerOwnerTask 的字段是包内私有的，包外既不能替换 repo，也不能替它接包装）。
//
// 本函数装配出的计算器与 NewCalcMinerOwnerTask 完全一致（同一组字段、同一段 Calc/save 逻辑），
// 差别只在「db → dal 仓储」这一步由调用方决定。生产路径继续走 NewCalcMinerOwnerTask。
func NewCalcMinerOwnerTaskWithDeps(conf *config.Config, repo repository.MinerTask,
	sg repository.SyncerGetter, luckCalc *luck.Calculator) *CalcMinerOwnerTask {
	return &CalcMinerOwnerTask{
		repo: repo,
		sg:   sg,
		conf: conf,
		luck: luckCalc,
	}
}
