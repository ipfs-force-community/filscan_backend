package fnscmd

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

// 派生表名（与 po.* 的 TableName 一致）
const (
	TableFNSEvents    = "fns.events"
	TableFNSTokens    = "fns.tokens"
	TableFNSActions   = "fns.actions"
	TableFNSTransfers = "fns.transfers"
	TableFNSReverses  = "fns.reverses"

	TableERC20Transfers = "fevm.erc_20_transfers"
	TableNFTTransfers   = "fevm.nft_transfers"
	TableNFTTokens      = "fevm.nft_tokens"
	TableABISignatures  = "fevm.abi_signatures"
)

// NoWriteFnsSaver 是 repository.FnsSaver 的「只统计不落库」包装。
// 它同时被 FnsTask（写 fns.events）与 CalcFnsTask（写 tokens/actions/transfers/reverses）复用，
// 因此计数器是「task + calculator」两条链路的写入之和。
type NoWriteFnsSaver struct {
	repository.FnsSaver

	events    *offlinereplay.Counters // fns.events
	tokens    *offlinereplay.Counters // fns.tokens
	actions   *offlinereplay.Counters // fns.actions
	transfers *offlinereplay.Counters // fns.transfers
	reverses  *offlinereplay.Counters // fns.reverses
}

var _ repository.FnsSaver = (*NoWriteFnsSaver)(nil)

// NewNoWriteFnsSaver 包装底层仓储（inner 必须非 nil：读操作需要它透传 —— 计算器要按高度读 fns.events）。
func NewNoWriteFnsSaver(inner repository.FnsSaver) *NoWriteFnsSaver {
	if inner == nil {
		panic("fnscmd: NewNoWriteFnsSaver 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteFnsSaver{
		FnsSaver:  inner,
		events:    offlinereplay.NewCounters(TableFNSEvents),
		tokens:    offlinereplay.NewCounters(TableFNSTokens),
		actions:   offlinereplay.NewCounters(TableFNSActions),
		transfers: offlinereplay.NewCounters(TableFNSTransfers),
		reverses:  offlinereplay.NewCounters(TableFNSReverses),
	}
}

// Inner 返回底层仓储（读操作透传目标）
func (r *NoWriteFnsSaver) Inner() repository.FnsSaver { return r.FnsSaver }

func (r *NoWriteFnsSaver) AddEvents(_ context.Context, items []*po.FNSEvent) error {
	r.events.CountWrite(len(items))
	return nil
}

func (r *NoWriteFnsSaver) AddToken(_ context.Context, item ...*po.FNSToken) error {
	r.tokens.CountWrite(len(item))
	return nil
}

func (r *NoWriteFnsSaver) AddAction(_ context.Context, _ *po.FNSAction) error {
	r.actions.CountWrite(1)
	return nil
}

func (r *NoWriteFnsSaver) AddTransfer(_ context.Context, _ *po.FNSTransfer) error {
	r.transfers.CountWrite(1)
	return nil
}

func (r *NoWriteFnsSaver) AddFNsReserveDomain(_ context.Context, _ *po.FnsReserve) error {
	r.reverses.CountWrite(1)
	return nil
}

func (r *NoWriteFnsSaver) AddFnsReserveDomainWithConflict(_ context.Context, _ *po.FnsReserve) error {
	r.reverses.CountWrite(1)
	return nil
}

// 以下删除方法只在回滚/清理路径出现（离线回放正常跑完不会走到），但同样必须拦下。
func (r *NoWriteFnsSaver) DeleteTokenByName(_ context.Context, _, _ string) error {
	r.tokens.CountDelete()
	return nil
}

func (r *NoWriteFnsSaver) DeleteEventsAfterEpoch(_ context.Context, _ chain.Epoch) error {
	r.events.CountDelete()
	return nil
}

func (r *NoWriteFnsSaver) DeleteTransferAfterEpoch(_ context.Context, _ chain.Epoch) error {
	r.transfers.CountDelete()
	return nil
}

func (r *NoWriteFnsSaver) DeleteActionsAfterEpoch(_ context.Context, _ chain.Epoch) error {
	r.actions.CountDelete()
	return nil
}

func (r *NoWriteFnsSaver) DeleteOriginReserve(_ context.Context, _, _ string) error {
	r.reverses.CountDelete()
	return nil
}

func (r *NoWriteFnsSaver) DeleteFnsReservesAfterEpoch(_ context.Context, _ chain.Epoch) error {
	r.reverses.CountDelete()
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteFnsSaver) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{
		r.events.Snapshot(),
		r.tokens.Snapshot(),
		r.actions.Snapshot(),
		r.transfers.Snapshot(),
		r.reverses.Snapshot(),
	}
}

// NoWriteFEvmRepo 是 repository.FEvmRepo 的「只统计不落库」包装：FnsTask 用它读签名（GetMethodNameBySignature /
// GetEventNameBySignature），写方法（erc20/nft 落库与 ABI 签名落库）一并拦下。
type NoWriteFEvmRepo struct {
	repository.FEvmRepo

	erc20Transfers *offlinereplay.Counters // fevm.erc_20_transfers
	nftTransfers   *offlinereplay.Counters // fevm.nft_transfers
	nftTokens      *offlinereplay.Counters // fevm.nft_tokens
	abiSignatures  *offlinereplay.Counters // fevm.abi_signatures
}

var _ repository.FEvmRepo = (*NoWriteFEvmRepo)(nil)

// NewNoWriteFEvmRepo 包装底层仓储（inner 必须非 nil：读操作需要它透传）。
func NewNoWriteFEvmRepo(inner repository.FEvmRepo) *NoWriteFEvmRepo {
	if inner == nil {
		panic("fnscmd: NewNoWriteFEvmRepo 需要非 nil 的底层仓储（读操作需透传给它）")
	}
	return &NoWriteFEvmRepo{
		FEvmRepo:       inner,
		erc20Transfers: offlinereplay.NewCounters(TableERC20Transfers),
		nftTransfers:   offlinereplay.NewCounters(TableNFTTransfers),
		nftTokens:      offlinereplay.NewCounters(TableNFTTokens),
		abiSignatures:  offlinereplay.NewCounters(TableABISignatures),
	}
}

// Inner 返回底层仓储（读操作透传目标）
func (r *NoWriteFEvmRepo) Inner() repository.FEvmRepo { return r.FEvmRepo }

func (r *NoWriteFEvmRepo) CreateERC20TransferBatch(_ context.Context, items []*po.FEvmERC20Transfer) error {
	r.erc20Transfers.CountWrite(len(items))
	return nil
}

func (r *NoWriteFEvmRepo) CreateErc721TransferBatch(_ context.Context, items []*po.NFTTransfer) error {
	r.nftTransfers.CountWrite(len(items))
	return nil
}

func (r *NoWriteFEvmRepo) CreateErc721Tokens(_ context.Context, items []*po.NFTToken) error {
	r.nftTokens.CountWrite(len(items))
	return nil
}

func (r *NoWriteFEvmRepo) SaveAPISignatures(_ context.Context, items []*po.FEvmABISignature) error {
	r.abiSignatures.CountWrite(len(items))
	return nil
}

// WriteStats 各派生表的写入统计快照（报告用）
func (r *NoWriteFEvmRepo) WriteStats() []offlinereplay.WriteStat {
	return []offlinereplay.WriteStat{
		r.erc20Transfers.Snapshot(),
		r.nftTransfers.Snapshot(),
		r.nftTokens.Snapshot(),
		r.abiSignatures.Snapshot(),
	}
}
