package acl

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	logging "github.com/gozelle/logger"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

var log = logging.NewLogger("acl")

type AggIndexAcl interface {
	FinalHeight(ctx context.Context) (epoch *chain.Epoch, err error)
	MinersBlockReward(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinersBlockReward, error)
	LatestTipset(ctx context.Context) ([]*londobell.Tipset, error)
	ActorStateEpoch(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) ([]*londobell.ActorStateEpoch, error)
	MinerInfo(ctx context.Context, epoch chain.Epoch, addr chain.SmartAddress) ([]*londobell.MinerInfo, error)
	MinerGasCost(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinerGasCost, error)
	BlockHeader(ctx context.Context, filters types.Filters) (result []*londobell.BlockHeader, err error)
	CountOfBlockMessages(ctx context.Context, start, end chain.Epoch) (count int64, err error)
	// RewardStreams 取 [start, end) 内 f02 奖励流的整点快照（contract A，londobell 仓）。
	RewardStreams(ctx context.Context, start, end chain.Epoch) ([]*londobell.RewardStream, error)
}

type AdapterIndexAcl interface {
	Actor(ctx context.Context, actorId chain.SmartAddress, epoch *chain.Epoch) (*londobell.ActorState, error)
	Miner(ctx context.Context, miner chain.SmartAddress, epoch *chain.Epoch) (*londobell.MinerDetail, error)
	CurrentSectorInitialPledge(ctx context.Context, epoch *chain.Epoch) (*londobell.CurrentSectorInitialPledge, error)
	Epoch(ctx context.Context, epoch *chain.Epoch) (*londobell.EpochReply, error)
}

func NewIndexAclImpl(agg AggIndexAcl, adapter AdapterIndexAcl, winCountRepo repository.WinCountReward) *IndexAclImpl {
	return &IndexAclImpl{agg: agg, adapter: adapter, winCountRepo: winCountRepo}
}

type IndexAclImpl struct {
	agg     AggIndexAcl
	adapter AdapterIndexAcl
	// winCountRepo 首页「每赢票奖励」实测口径的赢票数来源（PG chain.miner_win_counts）。
	winCountRepo repository.WinCountReward
}

func (a IndexAclImpl) CountOfBlockMessages(ctx context.Context, start, end chain.Epoch) (int64, error) {
	return a.agg.CountOfBlockMessages(ctx, start, end)
}

func (a IndexAclImpl) GetEpoch(ctx context.Context, epoch chain.Epoch) (*londobell.EpochReply, error) {
	return a.adapter.Epoch(ctx, &epoch)
}

func (a IndexAclImpl) GetAggLatestTipset(ctx context.Context) (tipset *londobell.Tipset, err error) {

	tipsets, err := a.agg.LatestTipset(ctx)
	if err != nil {
		return
	}

	if len(tipsets) > 0 {
		tipset = tipsets[0]
	} else {
		err = fmt.Errorf("agg latest tipset is empty")
		return
	}

	return
}

//func (a IndexAclImpl) GetBaseFee(ctx context.Context) (baseFee *decimal.Decimal, err error) {
//	tipset, err := a.agg.FinalHeight(ctx)
//	if err != nil {
//		return
//	}
//	var latestTipset *londobell.Tipset
//	if tipset != nil {
//		latestTipset = tipset[0]
//	}
//	baseFee = &latestTipset.BaseFee
//	return
//}

func (a IndexAclImpl) GetPowerIncrease24H(ctx context.Context, epoch chain.Epoch) (powerIncrease24H decimal.Decimal, err error) {

	end := epoch - 2880
	startPowerState, err := a.GetTotalEpochPower(ctx, epoch)
	if err != nil {
		return
	}
	endPowerState, err := a.GetTotalEpochPower(ctx, end)
	if err != nil {
		return
	}
	powerIncrease24H = startPowerState.TotalQualityAdjPower.Sub(endPowerState.TotalQualityAdjPower)
	return
}

func (a IndexAclImpl) GetTotalEpochPower(ctx context.Context, epoch chain.Epoch) (powerDetail *londobell.PowerActorDetail, err error) {
	addr := chain.SmartAddress("04")
	actors, err := a.adapter.Actor(ctx, addr, &epoch)
	if err != nil {
		return
	}
	if actors == nil {
		err = fmt.Errorf("epoch: %d(%s) agg get f04 is emtpy", epoch.Int64(), epoch.Format())
		return
	}

	actorState, err := json.Marshal(actors.State)
	if err != nil {
		return
	}

	powerDetail = new(londobell.PowerActorDetail)
	err = json.Unmarshal(actorState, &powerDetail)
	if err != nil {
		return
	}
	return
}

func (a IndexAclImpl) GetRewardIncrease24H(ctx context.Context, epoch chain.Epoch) (decimal.Decimal, error) {
	end := epoch - 2880
	startRewardState, err := a.GetTotalEpochReward(ctx, epoch)
	if err != nil {
		return decimal.Zero, err
	}
	endRewardState, err := a.GetTotalEpochReward(ctx, end)
	if err != nil {
		return decimal.Zero, err
	}
	// NV29(Solstice)：新状态里 TotalStoragePowerReward 改名且语义变为「全部铸造量」，
	// 用 MinerMinted() 统一成「矿工出块奖励」口径（= TotalMinted - TotalBurnMinted - TotalExplicitMinted）
	rewardIncrease24H := startRewardState.MinerMinted().Sub(endRewardState.MinerMinted())
	return rewardIncrease24H, err
}

func (a IndexAclImpl) GetTotalEpochReward(ctx context.Context, epoch chain.Epoch) (rewardDetail *londobell.RewardActorDetail, err error) {
	addr := chain.SmartAddress("02")
	actors, err := a.adapter.Actor(ctx, addr, &epoch)
	if err != nil {
		return
	}
	if actors == nil {
		err = fmt.Errorf("agg get actor f02 is empty")
		return
	}

	actorState, err := json.Marshal(actors.State)
	if err != nil {
		return
	}

	rewardDetail = new(londobell.RewardActorDetail)
	err = json.Unmarshal(actorState, &rewardDetail)
	if err != nil {
		return
	}

	return
}

// 首页「每赢票奖励」实测口径的窗口：最近 24h（2880 epoch，主网 30s/高度）。
const winCountRewardWindowEpochs int64 = 2880

// winCountRewardMinCoverage 窗口内赢票数**高度覆盖率**下限。低于此值认为本网该窗口数据不全，
// 回退旧口径（拆分前毛值）并打 WARN，而不是拿不完整的赢票数去算一个偏高的每赢票奖励。
const winCountRewardMinCoverage = 0.9

// RewardStreamDeltas24H 首页「近24h奖励三流」增量（attoFIL）。
//
// 全部取自 contract A /aggregators/reward_streams 窗口内整点快照的**首尾两行计数器差**：
//   - Miner   = ΔMinerMinted（NV29 起 = ΔTotalMintedReward − ΔTotalBurnMinted − ΔTotalExplicitMinted；
//     升级前即 ΔTotalStoragePowerReward）；
//   - Service = ΔTotalExplicitMinted（升级前恒 0）；Burn = ΔTotalBurnMinted（升级前恒 0）；
//   - Total   = ΔTotalMintedReward。NV29 起满足 Miner = Total − Service − Burn。
//
// OK=false 表示序列不足两行 / 首尾同一高度 / 全为空；此时四个金额均为零值，调用方按「无数据」处理。
type RewardStreamDeltas24H struct {
	Miner   decimal.Decimal
	Service decimal.Decimal
	Burn    decimal.Decimal
	Total   decimal.Decimal
	OK      bool
}

// rewardStreamDeltas24H 纯函数：由整点快照序列算首尾差分。任一端缺失 / 不足两行 / 首尾同高 ⇒ OK=false。
// 契约 A 已按 Epoch 升序返回，这里仍先排序再取首尾，避免上游顺序变化时取错端点。
func rewardStreamDeltas24H(streams []*londobell.RewardStream) RewardStreamDeltas24H {
	valid := make([]*londobell.RewardStream, 0, len(streams))
	for _, s := range streams {
		if s != nil {
			valid = append(valid, s)
		}
	}
	if len(valid) < 2 {
		return RewardStreamDeltas24H{}
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].Epoch < valid[j].Epoch })
	first, last := valid[0], valid[len(valid)-1]
	if first.Epoch == last.Epoch {
		return RewardStreamDeltas24H{}
	}
	return RewardStreamDeltas24H{
		Miner:   last.MinerMinted().Sub(first.MinerMinted()),
		Service: last.TotalExplicitMinted.Sub(first.TotalExplicitMinted),
		Burn:    last.TotalBurnMinted.Sub(first.TotalBurnMinted),
		Total:   last.TotalMintedReward.Sub(first.TotalMintedReward),
		OK:      true,
	}
}

// rewardStreamsWindow 取首页 24h 窗口 [T-2880, T] 的整点快照（end = T+1，把链头这一行也取进来）。
// 抽出来给 GetWinCountReward / GetRewardStreamDeltas24H / GetHomeRewardStreams24H 共用，
// 以保证首页一次请求对 aggregator 的 reward_streams 只调用一次。
func (a IndexAclImpl) rewardStreamsWindow(ctx context.Context, epoch chain.Epoch) ([]*londobell.RewardStream, error) {
	start := epoch - chain.Epoch(winCountRewardWindowEpochs)
	return a.agg.RewardStreams(ctx, start, epoch.Next())
}

// GetWinCountReward 首页「每赢票奖励」（薄封装）：先取一次 24h 奖励流快照，再交给
// winCountRewardFromStreams 复用取数结果。签名与全部兜底行为保持不变（测试/其它调用方沿用）。
func (a IndexAclImpl) GetWinCountReward(ctx context.Context, epoch chain.Epoch) (result decimal.Decimal, err error) {
	streams, streamsErr := a.rewardStreamsWindow(ctx, epoch)
	return a.winCountRewardFromStreams(ctx, epoch, streams, streamsErr)
}

// winCountRewardFromStreams 用已取好的整点快照算「每赢票奖励」（取数错误经 streamsErr 传入，不重复取数）。
//
// 实测口径：窗口内 Δ矿工实收 ÷ Δ赢票数，窗口＝最近 24h（2880 epoch）。
//   - Δ矿工实收 = f02 在 [T-2880, T] 两时点 MinerMinted() 之差，状态经 contract A 新接口
//     /aggregators/reward_streams 取（节点路径 /adapter/actor 只覆盖约 1 天、边界不稳，不用）。
//   - Δ赢票数 = PG chain.miner_win_counts 在 [T-2880, T) 内去重后的赢票总数。
//
// 兜底（都必须打日志，且**首页不得 500**）：
//   - 两时点计数器缺失 / 接口失败 → 回退旧口径 ThisEpochReward/5 + WARN；
//   - 分母为 0 → 返回 0；
//   - 窗口高度覆盖率 < 90% → 回退旧口径 + WARN。
func (a IndexAclImpl) winCountRewardFromStreams(ctx context.Context, epoch chain.Epoch, streams []*londobell.RewardStream, streamsErr error) (result decimal.Decimal, err error) {
	start := epoch - chain.Epoch(winCountRewardWindowEpochs)

	minerDelta, deltaOK := minerMintedDelta(streams)
	if streamsErr != nil || !deltaOK {
		log.Warnf("win_count_reward: reward_streams [%d,%d) 取数失败或首尾计数器缺失 (err=%v ok=%v)，回退旧口径 ThisEpochReward/5",
			start.Int64(), epoch.Int64(), streamsErr, deltaOK)
		return a.legacyWinCountReward(ctx, epoch)
	}

	sumWinCount, coveredEpochs, statsErr := a.winCountRepo.GetWinCountRewardStats(ctx, start, epoch)
	if statsErr != nil {
		log.Warnf("win_count_reward: 读 PG chain.miner_win_counts [%d,%d) 失败: %s，回退旧口径 ThisEpochReward/5",
			start.Int64(), epoch.Int64(), statsErr)
		return a.legacyWinCountReward(ctx, epoch)
	}

	// 分母为 0：直接给 0（不 panic，也不回退）。
	if sumWinCount == 0 {
		return decimal.Zero, nil
	}

	coverage := float64(coveredEpochs) / float64(winCountRewardWindowEpochs)
	if coverage < winCountRewardMinCoverage {
		log.Warnf("win_count_reward: 窗口赢票数高度覆盖不足 90%% [%d,%d): covered=%d/%d (%.1f%%)，回退旧口径 ThisEpochReward/5",
			start.Int64(), epoch.Int64(), coveredEpochs, winCountRewardWindowEpochs, coverage*100)
		return a.legacyWinCountReward(ctx, epoch)
	}

	result = minerDelta.Div(decimal.NewFromInt(sumWinCount))
	return result, nil
}

// GetRewardStreamDeltas24H 取一次 24h 奖励流快照并算三流增量（首尾差分）。供首页/统计复用。
// 取数失败时返回零值 deltas（OK=false）与非 nil error，调用方按「无数据」处理。
func (a IndexAclImpl) GetRewardStreamDeltas24H(ctx context.Context, epoch chain.Epoch) (RewardStreamDeltas24H, error) {
	streams, err := a.rewardStreamsWindow(ctx, epoch)
	return rewardStreamDeltas24H(streams), err
}

// HomeRewardStreams24H 首页「近24h奖励」一次取数的产物：每赢票奖励 + 三流明细。
//
// ⚠️ 为什么包成结构体而不直接返回两个值：BrowserBiz 会被 go-jsonrpc 反射注册成
// JSON-RPC 方法集，而 go-jsonrpc 的 processFuncOut（go-jsonrpc/util.go）只允许 0/1/2 个
// 返回值（2 个时第二个必须是 error）——3 个返回值会在**进程启动时 panic**，
// 编译不报错、单测也照样通过（实测踩过：cali 换件时新二进制启动即 exit 2）。
type HomeRewardStreams24H struct {
	WinCountReward decimal.Decimal
	Deltas         RewardStreamDeltas24H
}

// GetHomeRewardStreams24H 首页专用：**单次**取 24h 奖励流快照，同时产出「每赢票奖励」与三流明细，
// 保证首页一次请求对 aggregator 的 reward_streams 调用次数恒为 1（不因新增三流字段翻倍）。
//   - winCountReward 的兜底与 GetWinCountReward 完全一致（失败时 err 非 nil，由 biz 记日志）；
//   - deltas.OK=false（快照不足/取数失败）时四个金额为零值，由 biz 置 0，首页不 500、不 panic。
func (a IndexAclImpl) GetHomeRewardStreams24H(ctx context.Context, epoch chain.Epoch) (HomeRewardStreams24H, error) {
	streams, streamsErr := a.rewardStreamsWindow(ctx, epoch)
	res := HomeRewardStreams24H{Deltas: rewardStreamDeltas24H(streams)}
	wc, err := a.winCountRewardFromStreams(ctx, epoch, streams, streamsErr)
	res.WinCountReward = wc
	return res, err
}

// minerMintedDelta 返回奖励流序列首尾两行的「矿工实收」增量（attoFIL）。
// 任一端缺失 / 序列不足两行 / 首尾同一高度 ⇒ ok=false（调用方回退旧口径）。
func minerMintedDelta(streams []*londobell.RewardStream) (delta decimal.Decimal, ok bool) {
	d := rewardStreamDeltas24H(streams)
	if !d.OK {
		return decimal.Zero, false
	}
	return d.Miner, true
}

// legacyWinCountReward 旧口径：f02 当前 ThisEpochReward / 5。
// 这是区块奖励**拆分前**的毛值（NV29 后会越来越偏高），仅作兜底，不作为正常路径。
func (a IndexAclImpl) legacyWinCountReward(ctx context.Context, epoch chain.Epoch) (result decimal.Decimal, err error) {
	actorID := chain.SmartAddress("02")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return
		}
	}
	rewardState := londobell.RewardActorState{}
	err = json.Unmarshal(actorState, &rewardState)
	if err != nil {
		return
	}
	result = rewardState.ThisEpochReward.Div(decimal.NewFromInt(5))
	return
}

func (a IndexAclImpl) GetAvgBlockCount(ctx context.Context, epoch chain.Epoch) (result decimal.Decimal, err error) {
	var filters types.Filters
	endEpoch := epoch.CurrentHour()
	filters.End = &endEpoch
	startEpoch := chain.Epoch(filters.End.Int64() - 2880)
	filters.Start = &startEpoch
	blockList, err := a.agg.BlockHeader(ctx, filters)
	if err != nil {
		return
	}
	if blockList != nil {
		result = decimal.NewFromFloat(float64(len(blockList))).Div(decimal.NewFromFloat(2880))
	}
	return
}

func (a IndexAclImpl) GetAvgMessageCount(ctx context.Context, epoch chain.Epoch) (result int64, err error) {
	var filters types.Filters
	endEpoch := epoch.CurrentHour()
	filters.End = &endEpoch
	startEpoch := chain.Epoch(filters.End.Int64() - 2880)
	filters.Start = &startEpoch
	blockList, err := a.agg.BlockHeader(ctx, filters)
	if err != nil {
		return
	}
	if blockList != nil {
		var sumMessageCount int64
		for _, block := range blockList {
			sumMessageCount = sumMessageCount + block.MessageCount
		}
		result = sumMessageCount / 2880
	}
	return
}

type NetPower struct {
	QualityPower decimal.Decimal
	RawBytePower decimal.Decimal
}

func (a IndexAclImpl) GetTotalQualityPower(ctx context.Context, epoch chain.Epoch) (power NetPower, err error) {
	actorID := chain.SmartAddress("04")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return
		}
	}
	powerState := londobell.PowerActorState{}
	err = json.Unmarshal(actorState, &powerState)
	if err != nil {
		return
	}
	power.QualityPower = powerState.TotalQualityAdjPower
	power.RawBytePower = powerState.TotalRawBytePower
	return
}

// RewardStreamTotals 首页「累计奖励三股 + 累计铸造量」（attoFIL，f02 原始**累计计数器**，不是 24h 差分）。
//
// 口径（契约 A2）：
//   - Minted  = 累计铸造量：v19 = TotalMintedReward；v18 = 矿工实收（否则前端 minted 显示 0、与矿工行矛盾）；
//   - Miner   = 累计矿工实收：MinerMinted()（v18 即历史字段 TotalStoragePowerReward）；
//   - Service = 累计服务流（记在 f02、待受益方 Claim 提取）：TotalExplicitMinted，v18 恒 0；
//   - Burn    = 累计「铸造即销毁」：TotalBurnMinted，v18 恒 0。
//
// OK=false 表示 f02 状态取数/反序列化失败，此时四个金额均为零值，调用方按「无数据」处理（首页不 500）。
type RewardStreamTotals struct {
	Minted  decimal.Decimal
	Miner   decimal.Decimal
	Service decimal.Decimal
	Burn    decimal.Decimal
	OK      bool
}

// rewardStreamTotalsFromDetail 纯函数：由 f02 奖励 actor 状态算出累计三股。
// v18 判定用「三个 NV29 计数器全为 0」——结构体无 omitempty，v18 行这三项都是 "0"。
func rewardStreamTotalsFromDetail(d londobell.RewardActorDetail) RewardStreamTotals {
	miner := d.MinerMinted()
	if d.TotalMintedReward.IsZero() && d.TotalBurnMinted.IsZero() && d.TotalExplicitMinted.IsZero() {
		// NV29 之前：状态里只有 TotalStoragePowerReward（本身即矿工实收），无三计数器。
		return RewardStreamTotals{Minted: miner, Miner: miner, Service: decimal.Zero, Burn: decimal.Zero, OK: true}
	}
	return RewardStreamTotals{
		Minted:  d.TotalMintedReward,
		Miner:   miner,
		Service: d.TotalExplicitMinted,
		Burn:    d.TotalBurnMinted,
		OK:      true,
	}
}

// GetRewardStreamTotals 读 f02 奖励 actor 状态一次，产出累计三股 + 累计铸造量（首页用）。
//
// 与 GetTotalRewards 走**同一条** adapter.Actor(f02) 链路：首页若要同时用到 total_rewards 与这四个
// 新字段，只调用本方法一次即可，**不得再单独调 GetTotalRewards**（否则首页对节点的 f02 取数翻倍）。
func (a IndexAclImpl) GetRewardStreamTotals(ctx context.Context, epoch chain.Epoch) (totals RewardStreamTotals, err error) {
	actorID := chain.SmartAddress("02")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return RewardStreamTotals{}, err
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return RewardStreamTotals{}, err
		}
	}
	detail := londobell.RewardActorDetail{}
	if err = json.Unmarshal(actorState, &detail); err != nil {
		return RewardStreamTotals{}, err
	}
	return rewardStreamTotalsFromDetail(detail), nil
}

func (a IndexAclImpl) GetTotalRewards(ctx context.Context, epoch chain.Epoch) (totalRewards decimal.Decimal, err error) {
	// NV29(Solstice)：总奖励用「矿工出块奖励」口径（v18 = TotalStoragePowerReward，
	// v19 = TotalMinted − TotalBurn − TotalExplicit）；转发到 GetRewardStreamTotals 复用同一次取数链路。
	totals, err := a.GetRewardStreamTotals(ctx, epoch)
	if err != nil {
		return decimal.Zero, err
	}
	return totals.Miner, nil
}

func (a IndexAclImpl) GetActiveMiners(ctx context.Context, epoch chain.Epoch) (count int64, err error) {
	actorID := chain.SmartAddress("04")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	var actorState []byte
	if actor != nil {
		actorState, err = json.Marshal(actor.State)
		if err != nil {
			return
		}
	}
	powerState := londobell.PowerActorState{}
	err = json.Unmarshal(actorState, &powerState)
	if err != nil {
		return
	}
	count = powerState.MinerAboveMinPowerCount
	return
}

func (a IndexAclImpl) GetBurnt(ctx context.Context, epoch chain.Epoch) (burnt decimal.Decimal, err error) {
	actorID := chain.SmartAddress("099")
	actor, err := a.adapter.Actor(ctx, actorID, &epoch)
	if err != nil {
		return
	}
	burnt = actor.Balance
	return
}

func (a IndexAclImpl) GetMinerSectorSize(ctx context.Context, miner chain.SmartAddress) (sectorSize decimal.Decimal, err error) {
	var epoch *chain.Epoch
	actor, err := a.adapter.Miner(ctx, miner, epoch)
	if err != nil {
		return
	}
	var actorSectorSize int64
	if actor != nil {
		actorSectorSize = actor.SectorSize
	}
	decimalSectorSize := decimal.NewFromInt(actorSectorSize)
	sectorSize = decimalSectorSize
	return
}

func (a IndexAclImpl) GetInitialPledge(ctx context.Context) (initialPledge decimal.Decimal, err error) {
	sector, err := a.adapter.CurrentSectorInitialPledge(ctx, nil)
	if err != nil {
		return
	}
	var currentSectorInitialPledge decimal.Decimal
	if sector != nil {
		currentSectorInitialPledge = sector.CurrentSectorInitialPledge
	}
	decimalInitialPledge := currentSectorInitialPledge
	initialPledge = decimalInitialPledge
	return
}

func (a IndexAclImpl) GetCirculatingPercent(ctx context.Context) (circulatingPercent decimal.Decimal, err error) {
	sector, err := a.adapter.CurrentSectorInitialPledge(ctx, nil)
	if err != nil {
		return
	}
	var circulatingRate decimal.Decimal
	if sector != nil {
		circulatingRate = sector.CirculatingRate
	}
	circulatingPercent = circulatingRate
	return
}
