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
