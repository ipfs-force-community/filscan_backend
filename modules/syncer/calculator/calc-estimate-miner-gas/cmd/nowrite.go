package minergasestimatecmd

import (
	"context"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// NoWriteTraceRepo 是 repository.SyncerTraceTaskRepo 的「只统计不落库」包装：
// 写方法（chain.base_gas_costs 的两列更新、以及该仓储上其它表的增删）一律**不调用**底层仓储，
// 只往计数器记账；读方法（GetBaseGasCosts / GetLastBaseGasCostOrNil / ...）由嵌入接口原样透传。
//
// 安全性质：本类型的写方法体内不存在对底层仓储写方法的调用，因此即使计算器逻辑变化，
// 也不可能经由本类型改动链上任何一张派生表。
type NoWriteTraceRepo struct {
	// 嵌入底层仓储：读方法原样透传，只有写方法被本类型覆盖拦下
	repository.SyncerTraceTaskRepo

	baseGasCosts *offlinereplay.Counters // chain.base_gas_costs
	methodGas    *offlinereplay.Counters // chain.method_gas_fees
	minerGas     *offlinereplay.Counters // chain.miner_gas_fees
}

var _ repository.SyncerTraceTaskRepo = (*NoWriteTraceRepo)(nil)

// 派生表名（与 po 的 TableName 一致）
const (
	TableBaseGasCosts  = "chain.base_gas_costs"
	TableMethodGasFees = "chain.method_gas_fees"
	TableMinerGasFees  = "chain.miner_gas_fees"
)

// NewNoWriteTraceRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）
func NewNoWriteTraceRepo(inner repository.SyncerTraceTaskRepo) *NoWriteTraceRepo {
	if inner == nil {
		panic("minergasestimatecmd: NewNoWriteTraceRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteTraceRepo{
		SyncerTraceTaskRepo: inner,
		baseGasCosts:        offlinereplay.NewCounters(TableBaseGasCosts),
		methodGas:           offlinereplay.NewCounters(TableMethodGasFees),
		minerGas:            offlinereplay.NewCounters(TableMinerGasFees),
	}
}

// UpdateBaseGasCostSectorGas 拦下 chain.base_gas_costs 的扇区费更新（每次改 1 行），只计数
// —— 这是本计算器唯一的写方法
func (r *NoWriteTraceRepo) UpdateBaseGasCostSectorGas(_ context.Context, _ chain.Epoch, _, _ decimal.Decimal) error {
	r.baseGasCosts.CountWrite(1)
	return nil
}

// SaveBaseGasCost 拦下 chain.base_gas_costs 插入，只计数（本命令不跑 trace-task，正常不会走到）
func (r *NoWriteTraceRepo) SaveBaseGasCost(_ context.Context, _ *stat.BaseGasCost) error {
	r.baseGasCosts.CountWrite(1)
	return nil
}

// SaveMethodGasFees 拦下 chain.method_gas_fees 写入，只计数
func (r *NoWriteTraceRepo) SaveMethodGasFees(_ context.Context, entities []*po.MethodGasFee) error {
	r.methodGas.CountWrite(len(entities))
	return nil
}

// SaveMinerGasFees 拦下 chain.miner_gas_fees 写入，只计数
func (r *NoWriteTraceRepo) SaveMinerGasFees(_ context.Context, items []*po.MinerGasFee) error {
	r.minerGas.CountWrite(len(items))
	return nil
}

// DeleteBaseGasCosts 拦下 chain.base_gas_costs 删除（RollBack 路径），只计数
func (r *NoWriteTraceRepo) DeleteBaseGasCosts(_ context.Context, _ chain.Epoch) error {
	r.baseGasCosts.CountDelete()
	return nil
}

// DeleteMethodGasFees 拦下 chain.method_gas_fees 删除（RollBack 路径），只计数
func (r *NoWriteTraceRepo) DeleteMethodGasFees(_ context.Context, _ chain.Epoch) error {
	r.methodGas.CountDelete()
	return nil
}

// DeleteMinerGasFees 拦下 chain.miner_gas_fees 删除（RollBack 路径），只计数
func (r *NoWriteTraceRepo) DeleteMinerGasFees(_ context.Context, _ chain.Epoch) error {
	r.minerGas.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteTraceRepo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{
		r.baseGasCosts.Snapshot(),
		r.methodGas.Snapshot(),
		r.minerGas.Snapshot(),
	}
}
