package erc20cmd

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
)

// 派生表名（与 po.FEvmERC20Transfer / po.FEvmERC20Balance / po.FEvmERC20SwapInfo / po.FEvmERC20Contract 的
// TableName 一致）
const (
	TableERC20Transfers = "fevm.erc_20_transfers"
	TableERC20Balance   = "fevm.erc20_balance"
	TableERC20SwapInfo  = "fevm.erc20_swap_info"
	TableERC20Contract  = "fevm.erc20_contract"
)

// NoWriteERC20Repo 是 repository.ERC20TokenRepo 的「只统计不落库」包装：
// 写方法（新增/upsert/清理/更新合约）一律**不调用**底层仓储，只往计数器记账；读方法由嵌入接口透传。
//
// 安全性质：本类型的写方法体内不存在对底层仓储写方法的调用，因此即使上层 task 逻辑变化，
// 也不可能经由本类型写入 fevm.erc_20_transfers / fevm.erc20_balance / fevm.erc20_swap_info / fevm.erc20_contract。
type NoWriteERC20Repo struct {
	repository.ERC20TokenRepo

	transfers *offlinereplay.Counters // fevm.erc_20_transfers
	balances  *offlinereplay.Counters // fevm.erc20_balance
	swapInfos *offlinereplay.Counters // fevm.erc20_swap_info
	contracts *offlinereplay.Counters // fevm.erc20_contract（仅「新代币补扫」分支会写）
}

var _ repository.ERC20TokenRepo = (*NoWriteERC20Repo)(nil)

// NewNoWriteERC20Repo 包装底层仓储（inner 必须非 nil：读操作需要它透传）。
func NewNoWriteERC20Repo(inner repository.ERC20TokenRepo) *NoWriteERC20Repo {
	if inner == nil {
		panic("erc20cmd: NewNoWriteERC20Repo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteERC20Repo{
		ERC20TokenRepo: inner,
		transfers:      offlinereplay.NewCounters(TableERC20Transfers),
		balances:       offlinereplay.NewCounters(TableERC20Balance),
		swapInfos:      offlinereplay.NewCounters(TableERC20SwapInfo),
		contracts:      offlinereplay.NewCounters(TableERC20Contract),
	}
}

// Inner 返回底层仓储（读操作透传目标）
func (r *NoWriteERC20Repo) Inner() repository.ERC20TokenRepo { return r.ERC20TokenRepo }

func (r *NoWriteERC20Repo) CreateERC20TransferBatch(_ context.Context, items []*po.FEvmERC20Transfer) error {
	r.transfers.CountWrite(len(items))
	return nil
}

func (r *NoWriteERC20Repo) UpsertERC20BalanceBatch(_ context.Context, items []*po.FEvmERC20Balance) error {
	r.balances.CountWrite(len(items))
	return nil
}

func (r *NoWriteERC20Repo) CreateERC20SwapInfoBatch(_ context.Context, items []*po.FEvmERC20SwapInfo) error {
	r.swapInfos.CountWrite(len(items))
	return nil
}

func (r *NoWriteERC20Repo) UpdateOneERC20Contract(_ context.Context, _ string, _ *po.FEvmERC20Contract) error {
	r.contracts.CountWrite(1)
	return nil
}

func (r *NoWriteERC20Repo) CleanERC20TransferBatch(_ context.Context, _ int) error {
	r.transfers.CountDelete()
	return nil
}

func (r *NoWriteERC20Repo) CleanERC20SwapInfo(_ context.Context, _ int) error {
	r.swapInfos.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteERC20Repo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{
		r.transfers.Snapshot(),
		r.balances.Snapshot(),
		r.swapInfos.Snapshot(),
		r.contracts.Snapshot(),
	}
}
