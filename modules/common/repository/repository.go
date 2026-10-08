package repository

import (
	"context"
	"time"

	"github.com/filecoin-project/go-state-types/network"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/miner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/owner"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/stat"
	probo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type ActorGetter interface {
	GetActorByIdOrNil(ctx context.Context, nv network.Version, id actor.Id) (item *actor.Actor, err error)
	GetActorInfoByID(ctx context.Context, id actor.Id) (actor *bo.ActorInfo, err error)
}

type SyncEpochGetter interface {
	ChainEpoch(ctx context.Context) (epoch *chain.Epoch, err error)
	MinerEpoch(ctx context.Context) (epoch *chain.Epoch, err error)
	GetMinerEpochOrNil(ctx context.Context, epoch chain.Epoch) (result *chain.Epoch, err error)
}

type ActorAggRepo interface {
	ActorGetter
}

type SyncerTraceTaskRepo interface {
	ActorGetter
	GetLastBaseGasCostOrNil(ctx context.Context, epoch chain.Epoch) (base *stat.BaseGasCost, err error)
	SaveBaseGasCost(ctx context.Context, base *stat.BaseGasCost) (err error)
	SaveMethodGasFees(ctx context.Context, entities []*po.MethodGasFee) (err error)
	SaveMinerGasFees(ctx context.Context, items []*po.MinerGasFee) (err error)
	GetBaseGasCosts(ctx context.Context, gtStart, lteEnd chain.Epoch) (items []*po.BaseGasCostPo, err error)
	UpdateBaseGasCostSectorGas(ctx context.Context, epoch chain.Epoch, sectorFee32, sectorFee64 decimal.Decimal) (err error)
	DeleteBaseGasCosts(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteMinerGasFees(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteMethodGasFees(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

type MinerTask interface {
	SaveSyncMinerEpochPo(ctx context.Context, item *po.SyncMinerEpochPo) (err error)
	SaveMinerInfos(ctx context.Context, infos []*po.MinerInfo) (err error)
	SaveAbsPower(ctx context.Context, powerIncrease, powerLoss decimal.Decimal, epoch int64) error
	SaveOwnerInfos(ctx context.Context, infos []*po.OwnerInfo) (err error)
	SaveOwnerStats(ctx context.Context, stats []*po.OwnerStat) (err error)
	SaveMinerStats(ctx context.Context, stats []*po.MinerStat) (err error)
	GetMinerInfosByEpoch(ctx context.Context, epoch chain.Epoch) (entities []*po.MinerInfo, err error)
	GetOwnerInfosByEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.OwnerInfo, err error)
	GetMinersAccRewards(ctx context.Context, epochs chain.LORCRange) (rewards []*bo.AccReward, err error)
	GetMinersAccGasFees(ctx context.Context, epochs chain.LORCRange) (fees []*bo.AccGasFee, err error)
	GetMinersAccWinCount(ctx context.Context, epochs chain.LORCRange) (rewards []*bo.AccWinCount, err error)
	DeleteMinerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteOwnerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteAbsPower(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteSyncMinerEpochs(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteMinerStats(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteOwnerStats(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteMinerStatsBeforeEpoch(ctx context.Context, ltEpoch chain.Epoch) (err error)
	DeleteOwnerStatsBeforeEpoch(ctx context.Context, ltEpoch chain.Epoch) (err error)
}

type RewardTask interface {
	GetLastMinerRewardOrNil(ctx context.Context, epoch chain.Epoch, miner chain.SmartAddress) (item *miner.Reward, err error)
	GetLastOwnerRewardOrNil(ctx context.Context, epoch chain.Epoch, owner chain.SmartAddress) (item *owner.Reward, err error)
	SumRewards(ctx context.Context, epochs chain.LCRCRange) (values decimal.Decimal, err error)
	GetRewardMiners(ctx context.Context, epochs chain.LCRCRange) (miners []string, err error)
	SumMinersTotalRewards(ctx context.Context, miners []string) (aggRewards []*po.MinerAggReward, err error)
	GetNetQualityAdjPower(ctx context.Context, epoch chain.Epoch) (power decimal.Decimal, err error)
	SaveOwnerRewards(ctx context.Context, rewards []*owner.Reward) (err error)
	SaveMinerRewards(ctx context.Context, rewards []*miner.Reward) (err error)
	SaveWinCounts(ctx context.Context, winCounts []*po.MinerWinCount) (err error)
	SaveMinerAggReward(ctx context.Context, aggRewards []*po.MinerAggReward) (err error)
	SaveMinerRewardStats(ctx context.Context, stats []*po.MinerRewardStat) (err error)
	DeleteOwnerRewards(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteMinerRewards(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteWinCounts(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteMinerRewardStats(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

// RewardStreamRecipientTask 「奖励流受益方按高度快照」表 chain.reward_stream_recipient_epoch 的读写仓储
// （采集侧）：同步器每个高度用 Save 落一行/受益地址，回滚与历史清理各一个删除方法。
//
// 幂等约定：唯一键 (epoch, address)，Save 为批量 upsert（同 (epoch,address) 覆盖），
// 同一高度重跑不产生重复行。表结构与口径见 migration/37.reward_stream_recipient_epoch.sql。
type RewardStreamRecipientTask interface {
	// SaveRewardStreamRecipients 批量 upsert（同 (epoch, address) 覆盖）。
	// 空切片表示该高度没有受益方（不写、也不删旧行）：正常链上必有流，空多半是调用方判空后的省调用。
	SaveRewardStreamRecipients(ctx context.Context, items []*po.RewardStreamRecipientEpoch) (err error)
	// ListRewardStreamRecipientsByEpochRange 按 epoch 区间取（左闭右闭），
	// 供展示端聚合「本周期出现过的人」。按 (epoch asc, address asc) 定序返回。
	ListRewardStreamRecipientsByEpochRange(ctx context.Context, epochs chain.LCRCRange) (items []*po.RewardStreamRecipientEpoch, err error)
	// DeleteRewardStreamRecipientsGteEpoch 删除 epoch >= gteEpoch 的行（链回滚用）。
	DeleteRewardStreamRecipientsGteEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error)
	// DeleteRewardStreamRecipientsLteEpoch 删除 epoch <= lteEpoch 的行（历史清理 / HistoryClear 用）。
	DeleteRewardStreamRecipientsLteEpoch(ctx context.Context, lteEpoch chain.Epoch) (err error)
}

type MinerRewardRange interface {
	// MinerBlockRewardRange 逐 epoch 出块奖励（单矿工），区间左闭右开 [start, end)，
	// 对齐聚合器端点 miner_blockreward 的分组口径（按 epoch 分组，按 epoch 升序返回）。
	MinerBlockRewardRange(ctx context.Context, miner string, start, end chain.Epoch) (items []*bo.MinerEpochReward, err error)
	// MinersBlockRewardRange 逐 epoch 逐矿工出块奖励，区间左闭右开 [start, end)，
	// 对齐聚合器端点 miners_blockreward（按 epoch+miner 分组，按 epoch、miner 升序返回）。
	MinersBlockRewardRange(ctx context.Context, start, end chain.Epoch) (items []*bo.MinerEpochReward, err error)
	// MinerWinCountsRange 逐矿工 winCount + gasReward 区间汇总 [start, end)，
	// 对齐聚合器端点 wincount（按 miner 分组，按 miner 升序返回）。
	// TotalGasReward 对应 gas_reward 列（migration/36）；该列为 NULL 时 GasReward 为 nil，
	// 调用方必须回落聚合器（见 agg_pg_reward.go 的 incompleteGasRewardRow）。
	MinerWinCountsRange(ctx context.Context, start, end chain.Epoch) (items []*bo.AccWinCount, err error)
}

// LargeAmountTransfer 大额转账列表端点（/aggregators/transfer_message_for_largeAmount）
// 的 PG 读实现。该端点只吃 index/limit（没有高度区间），口径详见
// modules/common/infra/dal/dal_biz_large_transfer.go 的文件头注释。
type LargeAmountTransfer interface {
	// LargeTransfersPage 取一页大额转账，按 (epoch desc, cid asc) 定序；
	// offset/limit 由 dal.LargeTransferPageWindow(index, limit) 从请求的 index/limit 折算。
	// 空页返回 nil（调用方据此复现聚合器 data:null 的形态），不是错误。
	LargeTransfersPage(ctx context.Context, offset, limit int64) (items []*bo.LargeTransferRow, err error)
	// CountLargeTransfers 全表行数（= 聚合器 TotalCount 对照值）。
	// 关键口径：**count(*) 不去重**、**不按高度区间过滤**（表不存区间，请求也不带区间），
	// 见 dal 注释里的三条理由。
	CountLargeTransfers(ctx context.Context) (total int64, err error)
}

type BaseFeeTrendBizRepo interface {
	GetStatBaseGasCost(ctx context.Context, epochs []chain.Epoch) (costs []*stat.BaseGasCost, err error)
}

type ContractTrendBizRepo interface {
	GetContractUsersByEpochs(ctx context.Context, points []chain.Epoch) (items []*filscan.ContractUsersTrend, err error)
	GetContractCntByEpochs(ctx context.Context, points []chain.Epoch) (items []*bo.ContractCnt, err error)
	GetContractTxsByEpochs(ctx context.Context, points []chain.Epoch) (items []*filscan.ContractTxsTrend, err error)
	GetContractBalanceByEpochs(ctx context.Context, points []chain.Epoch) (items []*filscan.ContractBalanceTrend, err error)
}

type Gas24hTrendBizRepo interface {
	GetLatestMethodGasCostEpoch(ctx context.Context) (epoch chain.Epoch, err error)
	GetMethodGasFees(ctx context.Context, epochs chain.LCRORange) (costs []*po.MethodGasFee, err error)
}

type BaselineTaskRepo interface {
	GetLatestBuiltinActorHeight(ctx context.Context) (epoch chain.Epoch, err error)
	SaveBuiltActorStates(ctx context.Context, item ...*po.BuiltinActorStatePo) (err error)
	DeleteBuiltActorStates(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

type StatisticBaseLineBizRepo interface {
	GetBaseLinePowerByPoints(ctx context.Context, points []chain.Epoch) (entities []*bo.BaseLinePower, err error)
}

type OwnerRankBizRepo interface {
	GetOwnerRanks(ctx context.Context, epoch chain.Epoch, query filscan.PagingQuery) (items []*bo.OwnerRank, total int64, err error)
}

type AbsPower interface {
	GetPowerAbs(ctx context.Context, start, end int64) ([]*po.AbsPowerChange, error)
	GetMaxEpochInPowerAbs(ctx context.Context) (int64, error)
}

type MinerRankBizRepo interface {
	GetMinerRanks(ctx context.Context, epoch chain.Epoch, query filscan.PagingQuery) (items []*bo.MinerRank, total int64, err error)
	GetMinerPowerRanks(ctx context.Context, epoch, compare chain.Epoch, sectorSize uint64, query filscan.PagingQuery) (items []*bo.MinerPowerRank, total int64, err error)
	GetMinerRewardRanks(ctx context.Context, interval string, epoch chain.Epoch, sectorSize uint64, query filscan.PagingQuery) (items []*bo.MinerRewardRank, total int64, err error)
}

type StatisticBlockRewardTrendBizRepo interface {
	GetBlockRewardsByEpochs(ctx context.Context, interval string, points []int64) (items []*bo.SumMinerReward, err error)
}

type StatisticActiveMinerTrendBizRepo interface {
	GetActiveMinerCountsByEpochs(ctx context.Context, points []chain.Epoch) (items []*bo.ActiveMinerCount, err error)
}

type StatisticMessageCountTrendBizRepo interface {
	GetMessageCountsByEpochs(ctx context.Context, points []chain.Epoch) (items []*bo.MessageCount, err error)
}

// WinCountReward 首页「每赢票奖励」实测口径（窗口 Δ矿工实收 ÷ Δ赢票数）的赢票数读实现。
// 只读 chain.miner_win_counts（同步器 reward-task 写；本仓已有）。
type WinCountReward interface {
	// GetWinCountRewardStats 返回 [start, end) 内**去重后**的赢票总数，以及该区间内有赢票数据的高度数
	// （供调用方算高度覆盖率：coveredEpochs / 窗口高度数）。区间左闭右开。
	GetWinCountRewardStats(ctx context.Context, start, end chain.Epoch) (sumWinCount, coveredEpochs int64, err error)
}

type OwnerGetterRepo interface {
	IsOwner(ctx context.Context, addr chain.SmartAddress) (ok bool, err error)
}

type MinerGetterRepo interface {
	IsMiner(ctx context.Context, addr chain.SmartAddress) (ok bool, err error)
}

type ActorBalanceTaskRepo interface {
	GetRichAccountRank(ctx context.Context, query filscan.PagingQuery) (result *bo.RichAccountRankList, err error)
	SaveActorsBalance(ctx context.Context, actorsBalance []*actor.RichActor) (err error)
	DeleteActorsBalance(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

type ActorTypeTaskRepo interface {
	SaveActorsType(ctx context.Context, actorsType []*actor.ActorsType) (err error)
	GetActorType(ctx context.Context, actorID chain.SmartAddress) (result *bo.ActorType, err error)
}

type GasPerTRepo interface {
	GetGasPerT(ctx context.Context) (result *bo.GasPerT, err error)
}

type BannerIndicatorRepo interface {
	GetMinerPowerProportion(ctx context.Context) (result []*bo.MinerCount, err error)
	GetTotalBalance(ctx context.Context) (res *decimal.Decimal, err error)
}

type GetAddrTagRepo interface {
	GetAllAddrTags(ctx context.Context) ([]*po.AddressTag, error)
}

type ChangeActorTask interface {
	GetExistsActors(ctx context.Context, ids []string) (items map[string]string, err error)
	GetActorsByIds(ctx context.Context, ids []string) (items []*po.ActorPo, err error)
	GetActorById(ctx context.Context, id string) (item *po.ActorPo, err error)
	GetActorByRobust(ctx context.Context, robust string) (item *po.ActorPo, err error)
	GetActorBalances(ctx context.Context, epoch chain.Epoch) (items []*po.ActorBalance, err error)
	AddActorBalances(ctx context.Context, balances []*po.ActorBalance) (err error)
	AddActors(ctx context.Context, actors []*po.ActorPo) (err error)
	AddActorActions(ctx context.Context, actors []*po.ActorAction) (err error)
	GetActorActionsAfterEpoch(ctx context.Context, gteEpoch chain.Epoch) (actions []*po.ActorAction, err error)
	GetMinerSizeOrZero(ctx context.Context, miner string) (size int64, err error)
	DeleteActorsByIds(ctx context.Context, ids []string) (err error)
	DeleteActorBalances(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteActorActions(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

type ActorBalanceTrendBizRepo interface {
	GetActorBalanceTrend(ctx context.Context, actorID actor.Id, start chain.Epoch, points []chain.Epoch) (actorBalanceTrend []*bo.ActorBalanceTrend, err error)
	GetLatestEpoch(ctx context.Context) (epoch *chain.Epoch, err error)
	GetActorUnderEpochBalance(ctx context.Context, actorID actor.Id, start chain.Epoch) (actorBalanceTrend *bo.ActorBalanceTrend, err error)
}

type MinerLocationTaskRepo interface {
	GetLatestMinerMultiAddrs(ctx context.Context) (addrs []*bo.MinerIpAddr, err error)
	CleanMinerLocations(ctx context.Context, powerMiners []string) (err error)
	SaveMinerLocation(ctx context.Context, item *po.MinerLocation) (err error)
	GetUpdateMinerLocations(ctx context.Context, before time.Time, limit int64) (locations []*po.MinerLocation, err error)
	UpdateMinerIp(ctx context.Context, item *po.MinerLocation) (err error)
}

type MessageCountTaskRepo interface {
	SaveMessageCounts(ctx context.Context, count *po.MessageCount) (err error)
	GetAvgBlockCount24h(ctx context.Context) (count decimal.Decimal, err error)
	DeleteMessageCounts(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

type ActorSyncSaver interface {
	GetNoneCreatedTimeActors(ctx context.Context) (items []*po.ActorPo, err error)
	UpdateActorCreateTime(ctx context.Context, item *po.ActorPo) (err error)
}

type DealProposalTaskRepo interface {
	GetCidByDeal(ctx context.Context, dealID int64) (item *po.DealProposalPo, err error)
	SaveDealProposals(ctx context.Context, items ...*po.DealProposalPo) (err error)
	DeleteDealProposals(ctx context.Context, gteEpoch chain.Epoch) (err error)
}

type FEvmRepo interface {
	CreateERC20TransferBatch(ctx context.Context, items []*po.FEvmERC20Transfer) (err error)
	CreateErc721TransferBatch(ctx context.Context, items []*po.NFTTransfer) (err error)
	CreateErc721Tokens(ctx context.Context, items []*po.NFTToken) (err error)
	SaveAPISignatures(ctx context.Context, items []*po.FEvmABISignature) (err error)
	GetMethodNameBySignature(ctx context.Context, sig string) (name string, err error)
	GetEventNameBySignature(ctx context.Context, sig string) (name string, err error)
}

type ERC20TokenRepo interface {
	GetERC20TransferInMessage(ctx context.Context, cid string) ([]*po.FEvmERC20Transfer, error)
	GetERC20TransferByContract(ctx context.Context, contractID string, page, limit int) (int64, []*po.FEvmERC20Transfer, error)
	GetERC20TransferByRelatedAddr(ctx context.Context, addr, tokenName string, page, limit int) (int64, []*po.FEvmERC20Transfer, error)
	GetERC20TransferTokenNamesByRelatedAddr(ctx context.Context, addr string) ([]string, error)
	GetERC20TransferInDexByContract(ctx context.Context, contractID string, page, limit int) (int64, []*po.FEvmERC20Transfer, error)
	GetERC20SwapInfoByContract(ctx context.Context, contractID string, page, limit int) (int64, []*po.FEvmERC20SwapInfo, error)
	GetERC20SwapInfoByCid(ctx context.Context, cid string) (*po.FEvmERC20SwapInfo, error)
	GetERC20BalanceByContract(ctx context.Context, contractID, filter string, page, limit int) (int64, []*po.FEvmERC20Balance, error)
	GetUniqueTokenHolderByContract(ctx context.Context, contractID string) (int64, error)
	GetUniqueNoneZeroTokenHolderByContract(ctx context.Context, contractID string) (int64, error)
	GetAllERC20Contracts(ctx context.Context) ([]*po.FEvmERC20Contract, error)
	GetMethodsDecodeSignature(ctx context.Context, hex string) (string, error)
	GetAllMethodsDecodeSignature(ctx context.Context) ([]po.FEvmMethods, error)
	GetOneERC20Contract(ctx context.Context, contractID string) (*po.FEvmERC20Contract, error)
	GetERC20AmountOfOneAddress(ctx context.Context, address string) ([]*po.FEvmERC20Balance, error)
	GetEvmEventSignatures(ctx context.Context, hexSignature []string) (signature []*po.EvmEventSignature, err error)
	CreateERC20TransferBatch(ctx context.Context, items []*po.FEvmERC20Transfer) (err error)
	UpsertERC20BalanceBatch(ctx context.Context, items []*po.FEvmERC20Balance) (err error)
	GetERC20TransferBatchAfterEpochInOneContract(ctx context.Context, contractId string, epoch, limit, page int) (int64, []*po.FEvmERC20Transfer, error)
	CreateERC20SwapInfoBatch(ctx context.Context, items []*po.FEvmERC20SwapInfo) (err error)
	CleanERC20TransferBatch(ctx context.Context, epoch int) error
	CleanERC20SwapInfo(ctx context.Context, epoch int) error
	GetERC20TransferBatchAfterEpoch(ctx context.Context, epoch int) ([]*po.FEvmErc20Transfer, error)
	GetAllERC20FreshContracts(ctx context.Context) ([]*po.FEvmErc20FreshList, error)
	UpdateOneERC20Contract(ctx context.Context, contractID string, contract *po.FEvmERC20Contract) error
	GetUniqueContractsInTransfers(ctx context.Context) ([]string, error)
	GetContractsUrl(ctx context.Context, contracts []string) ([]*po.ContractIcons, error)
	GetDexInfo(ctx context.Context, contractID string) (*po.DexInfo, error)
}

type EvmContractRepo interface {
	SaveFEvmContracts(ctx context.Context, item *po.FEvmContracts) (err error)
	SelectFEvmContractsByActorID(ctx context.Context, actorID string) (item *po.FEvmContracts, err error)
	SaveFEvmContractSols(ctx context.Context, item []*po.FEvmContractSols) (err error)
	SelectFEvmContractSolsByActorID(ctx context.Context, actorID string) (item []*po.FEvmContractSols, err error)
	SelectFEvmMainContractByActorID(ctx context.Context, actorID string) (item *po.FEvmContractSols, err error)
	SelectVerifiedFEvmContracts(ctx context.Context, page *int, limit *int) (items []*bo.VerifiedContracts, count int64, err error)
}

type EvmTransferRepo interface {
	GetEvmTransferStatsByContractName(ctx context.Context, contractName string) (evmTransfer *po.EvmTransferStat, err error)
	GetEvmTransferStatsByID(ctx context.Context, actorID string) (evmTransfer *po.EvmTransferStat, err error)
	GetEvmTransferByID(ctx context.Context, actorID string) (evmTransfer *bo.EVMTransferStatsWithName, err error)
	GetEvmTransferStatsList(ctx context.Context, page, limit int, filed, sort, interval string) (transfers []*po.EvmTransferStat, count int, err error)
	GetEvmTransferList(ctx context.Context, epochs *chain.LORCRange, page, limit int, filed, sort, interval string) (actors []*bo.EvmTransfers, count int, err error)
	SaveEvmTransfers(ctx context.Context, infos []*po.EvmTransfer) (err error)
	DeleteEvmTransfers(ctx context.Context, gteEpoch chain.Epoch) (err error)
	GetEvmTransferStats(ctx context.Context, epoch chain.Epoch) (accTransfer []*bo.EVMTransferStats, err error)
	SaveEvmTransferStats(ctx context.Context, infos []*po.EvmTransferStat) (err error)
	DeleteEvmTransferStats(ctx context.Context, gteEpoch chain.Epoch) (err error)
	CountUniqueContracts(ctx context.Context, epoch chain.Epoch) (int64, error)
	CountTxsOfContracts(ctx context.Context, epoch chain.Epoch) (int64, error)
	GetTxsOfContractsByRange(ctx context.Context, start, end chain.Epoch) ([]*po.EvmTransfer, error)
	CountUniqueUsers(ctx context.Context, epoch chain.Epoch) (int64, error)
	CountVerifiedContracts(ctx context.Context) (int64, error)
}

type EvmTransactionRepo interface {
	GetEvmTransactionStatsByID(ctx context.Context, actorID string) (evmTransaction *po.EvmTransactionStat, err error)
	GetEvmTransactionStatsList(ctx context.Context, page, limit int, filed, sort, interval string) (Transactions []*po.EvmTransactionStat, count int, err error)
	GetEvmTransactions(ctx context.Context, epochs chain.LCRCRange) (items []*po.EvmTransaction, err error)
	GetEvmTransactionsAfterEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.EvmTransaction, err error)
	SaveEvmTransactions(ctx context.Context, infos []*po.EvmTransaction) (err error)
	DeleteEvmTransactions(ctx context.Context, gteEpoch chain.Epoch) (err error)
	DeleteEvmTransactionsBeforeEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error)
	GetEvmTransactionStats(ctx context.Context) (accTransaction []*bo.EVMTransactionStats, err error)
	SaveEvmTransactionStats(ctx context.Context, infos []*po.EvmTransactionStat) (err error)
	DeleteEvmTransactionStats(ctx context.Context, gteEpoch chain.Epoch) (err error)
	SaveEvmTransactionUser(ctx context.Context, infos *po.EvmTransactionUser) (err error)
}

type EvmSignatureRepo interface {
	SaveEvmEventSignatures(ctx context.Context, infos []*po.EvmEventSignature) (err error)
	GetEvmEventSignatures(ctx context.Context, hexSignature []string) (signature []*po.EvmEventSignature, err error)
}

type DefiRepo interface {
	BatchSaveDefiItems(ctx context.Context, items []*po.DefiDashboard) error
	GetDefiItems(ctx context.Context, page, limit int) (int64, []*po.DefiDashboard, error)
	CleanDefiItems(ctx context.Context, gteEpoch chain.Epoch) (err error)
	GetMaxHeight(ctx context.Context) (int64, error)
	GetAllItemsOnEpoch(ctx context.Context, epoch int64) ([]*po.DefiDashboard, error)
	GetItemsInRange(ctx context.Context, epoch int64) (int, []*po.DefiDashboard, error)
	GetProductMainSite(ctx context.Context, contractId string) string
	GetMaxHeight24hTvl(ctx context.Context) (decimal.Decimal, decimal.Decimal, error)
	GetTvlByEpochs(ctx context.Context, epochs []chain.Epoch) ([]*bo.DefiTvl, error)
	ERC20TokenRepo
}

type ResourceRepo interface {
	GetBannerByCategoryAndLanguage(ctx context.Context, category, language string) ([]*po.Banner, error)

	GetFEvmItemsByCategory(ctx context.Context, category string) ([]*po.FEvmItem, []*po.FEvmItemCategory, error)
	GetFEvmCategorys(ctx context.Context) ([]string, []int, error)
	GetHotItems(ctx context.Context) ([]*po.FEvmItem, []*po.FEvmHotItem, error)
}

type FilPriceRepo interface {
	SaveFilPrice(ctx context.Context, price, percentChange float64, time time.Time) error
	LatestPrice(ctx context.Context) (*po.FilPrice, error)
}

type StatisticDcTrendBizRepo interface {
	QueryDCPowers(ctx context.Context, epochs []int64) (items []*bo.DCPower, err error)
}

type EventsRepo interface {
	GetEventsList(ctx context.Context) (items []*po.Events, err error)
}

type InviteCodeRepo interface {
	GetUserInviteCode(ctx context.Context, userID int) (item po.InviteCode, err error)
	SaveUserInviteCode(ctx context.Context, userID int, code string) (err error)
	GetUserIDByInviteCode(ctx context.Context, code string) (int, error)

	SaveUserInviteRecord(ctx context.Context, userID int, code, email string, createAt time.Time) (err error)
	GetUserInviteRecordByCode(ctx context.Context, code string) (items []*po.UserInviteRecord, err error)
	GetUserInviteRecordByUserID(ctx context.Context, userID int) (item po.UserInviteRecord, err error)
	UpdateUserIsValid(ctx context.Context, userID int64) error

	GetInviteSuccessRecord(ctx context.Context, userID int64) (bool, error)
	SaveSuccessRecords(ctx context.Context, userID int64) error
}

type CapitalRepo interface {
	GetAddressRank(ctx context.Context) (result *probo.RichAccountRankList, err error)
	GetLatestBalanceBeforeEpoch(ctx context.Context, address string, epoch *chain.Epoch) (balance decimal.Decimal, err error)
	GetLatestBalanceAfterEpoch(ctx context.Context, address string, epoch *chain.Epoch) (balance decimal.Decimal, err error)
}

// LargeTransferRepo 大额转账预计算表 chain.large_transfers 的**增量维护**仓储（写入侧）。
//
// 实现约定（唯一的维护入口，幂等与失败策略都靠它）：
//   - 没有主键、不能建唯一索引（线上口径要在同一高度同一 cid 上保留 trace 行重数），
//     所以不做 upsert；ReplaceLargeTransfers 必须在**同一个事务**里先 delete 后批量 insert，
//     否则崩溃/回滚会留下半截或重复行；
//   - 同一高度重复调用（重试、回放）后的表内容必须与只调一次完全相同（delete-then-insert 天然满足）。
type LargeTransferRepo interface {
	// ReplaceLargeTransfers 用 items 整体替换该高度已有的行（先按 epoch 删除，再批量插入，同一事务）。
	// items 为空表示「该高度没有大额转账」：仍然要执行删除（清掉该高度可能残留的旧行），只是不插入。
	ReplaceLargeTransfers(ctx context.Context, epoch int64, items []*po.LargeTransfer) (err error)
	// DeleteLargeTransfersFromEpoch 删除 >= gteEpoch 的行（链回滚时用；重跑会按 delete-then-insert 重写）。
	DeleteLargeTransfersFromEpoch(ctx context.Context, gteEpoch chain.Epoch) (err error)
}
