package minerownercmd

import (
	"context"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// NoWriteMinerTaskRepo 是 repository.MinerTask 的「只统计不落库」包装：
// 写方法一律**不调用**底层仓储，只往计数器记账；读方法由嵌入接口原样透传。
//
// 覆盖范围刻意取满整个接口（不只是计算器会走的那三个写方法）：
//   - 计算器 calc-miner-owner-task 会写的：SaveSyncMinerEpochPo / SaveMinerStats / SaveOwnerStats；
//   - 计算器 RollBack 会删的：DeleteSyncMinerEpochs / DeleteMinerStats / DeleteOwnerStats；
//   - miner-task 的写方法（SaveMinerInfos / SaveOwnerInfos / SaveAbsPower 及各自的删除）：
//     本命令不注册 miner-task，正常不会走到，但一并拦下后，「--no-write 下一个字节都写不出去」
//     不再依赖「当前注册了哪些任务」这个前提。
//
// 安全性质：本类型的写方法体内不存在对底层仓储写方法的调用，因此即使计算器逻辑变化，
// 也不可能经由本类型写入 chain.sync_miner_epochs / chain.miner_stats / chain.owner_stats
// 以及 chain.miner_infos / chain.owner_infos / chain.abs_power_change。
type NoWriteMinerTaskRepo struct {
	// 嵌入底层仓储：读方法原样透传，只有写方法被本类型覆盖拦下
	repository.MinerTask

	syncMinerEpochs *offlinereplay.Counters // chain.sync_miner_epochs
	minerStats      *offlinereplay.Counters // chain.miner_stats
	ownerStats      *offlinereplay.Counters // chain.owner_stats
	minerInfos      *offlinereplay.Counters // chain.miner_infos
	ownerInfos      *offlinereplay.Counters // chain.owner_infos
	absPower        *offlinereplay.Counters // chain.abs_power_change
}

var _ repository.MinerTask = (*NoWriteMinerTaskRepo)(nil)

// 派生表名（与 po 的 TableName 一致）
const (
	TableSyncMinerEpochs = "chain.sync_miner_epochs"
	TableMinerStats      = "chain.miner_stats"
	TableOwnerStats      = "chain.owner_stats"
	TableMinerInfos      = "chain.miner_infos"
	TableOwnerInfos      = "chain.owner_infos"
	TableAbsPowerChange  = "chain.abs_power_change"
)

// NewNoWriteMinerTaskRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）。
func NewNoWriteMinerTaskRepo(inner repository.MinerTask) *NoWriteMinerTaskRepo {
	if inner == nil {
		panic("minerownercmd: NewNoWriteMinerTaskRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteMinerTaskRepo{
		MinerTask:       inner,
		syncMinerEpochs: offlinereplay.NewCounters(TableSyncMinerEpochs),
		minerStats:      offlinereplay.NewCounters(TableMinerStats),
		ownerStats:      offlinereplay.NewCounters(TableOwnerStats),
		minerInfos:      offlinereplay.NewCounters(TableMinerInfos),
		ownerInfos:      offlinereplay.NewCounters(TableOwnerInfos),
		absPower:        offlinereplay.NewCounters(TableAbsPowerChange),
	}
}

// SaveSyncMinerEpochPo 拦下 chain.sync_miner_epochs 写入（计算器 save() 的台账行），只计数。
// 这一行就是页面「该整点是否算完」的判据。
func (r *NoWriteMinerTaskRepo) SaveSyncMinerEpochPo(_ context.Context, item *po.SyncMinerEpochPo) error {
	rows := 0
	if item != nil {
		rows = 1
	}
	r.syncMinerEpochs.CountWrite(rows)
	return nil
}

// SaveMinerStats 拦下 chain.miner_stats 写入，只计数（行数 = 本次本会写入的行数）
func (r *NoWriteMinerTaskRepo) SaveMinerStats(_ context.Context, stats []*po.MinerStat) error {
	r.minerStats.CountWrite(len(stats))
	return nil
}

// SaveOwnerStats 拦下 chain.owner_stats 写入，只计数
func (r *NoWriteMinerTaskRepo) SaveOwnerStats(_ context.Context, stats []*po.OwnerStat) error {
	r.ownerStats.CountWrite(len(stats))
	return nil
}

// SaveMinerInfos 拦下 chain.miner_infos 写入，只计数（本命令不跑 miner-task，正常不会走到）
func (r *NoWriteMinerTaskRepo) SaveMinerInfos(_ context.Context, infos []*po.MinerInfo) error {
	r.minerInfos.CountWrite(len(infos))
	return nil
}

// SaveOwnerInfos 拦下 chain.owner_infos 写入，只计数（本命令不跑 miner-task，正常不会走到）
func (r *NoWriteMinerTaskRepo) SaveOwnerInfos(_ context.Context, infos []*po.OwnerInfo) error {
	r.ownerInfos.CountWrite(len(infos))
	return nil
}

// SaveAbsPower 拦下 chain.abs_power_change 写入，只计数（本命令不跑 miner-task，正常不会走到）
func (r *NoWriteMinerTaskRepo) SaveAbsPower(_ context.Context, _, _ decimal.Decimal, _ int64) error {
	r.absPower.CountWrite(1)
	return nil
}

// DeleteSyncMinerEpochs 拦下 chain.sync_miner_epochs 删除（RollBack 路径），只计数
func (r *NoWriteMinerTaskRepo) DeleteSyncMinerEpochs(_ context.Context, _ chain.Epoch) error {
	r.syncMinerEpochs.CountDelete()
	return nil
}

// DeleteMinerStats 拦下 chain.miner_stats 删除（RollBack 路径），只计数
func (r *NoWriteMinerTaskRepo) DeleteMinerStats(_ context.Context, _ chain.Epoch) error {
	r.minerStats.CountDelete()
	return nil
}

// DeleteOwnerStats 拦下 chain.owner_stats 删除（RollBack 路径），只计数
func (r *NoWriteMinerTaskRepo) DeleteOwnerStats(_ context.Context, _ chain.Epoch) error {
	r.ownerStats.CountDelete()
	return nil
}

// DeleteMinerStatsBeforeEpoch 拦下 chain.miner_stats 的历史清理删除，只计数
func (r *NoWriteMinerTaskRepo) DeleteMinerStatsBeforeEpoch(_ context.Context, _ chain.Epoch) error {
	r.minerStats.CountDelete()
	return nil
}

// DeleteOwnerStatsBeforeEpoch 拦下 chain.owner_stats 的历史清理删除，只计数
func (r *NoWriteMinerTaskRepo) DeleteOwnerStatsBeforeEpoch(_ context.Context, _ chain.Epoch) error {
	r.ownerStats.CountDelete()
	return nil
}

// DeleteMinerInfos 拦下 chain.miner_infos 删除，只计数（miner-task 路径，正常不会走到）
func (r *NoWriteMinerTaskRepo) DeleteMinerInfos(_ context.Context, _ chain.Epoch) error {
	r.minerInfos.CountDelete()
	return nil
}

// DeleteOwnerInfos 拦下 chain.owner_infos 删除，只计数（miner-task 路径，正常不会走到）
func (r *NoWriteMinerTaskRepo) DeleteOwnerInfos(_ context.Context, _ chain.Epoch) error {
	r.ownerInfos.CountDelete()
	return nil
}

// DeleteAbsPower 拦下 chain.abs_power_change 删除，只计数（miner-task 路径，正常不会走到）
func (r *NoWriteMinerTaskRepo) DeleteAbsPower(_ context.Context, _ chain.Epoch) error {
	r.absPower.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteMinerTaskRepo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{
		r.syncMinerEpochs.Snapshot(),
		r.minerStats.Snapshot(),
		r.ownerStats.Snapshot(),
		r.minerInfos.Snapshot(),
		r.ownerInfos.Snapshot(),
		r.absPower.Snapshot(),
	}
}
