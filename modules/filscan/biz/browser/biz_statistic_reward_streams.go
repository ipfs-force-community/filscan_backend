package browser

import (
	"context"
	"sort"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件：统计页新接口「区块奖励流向」（NV29 / FIP-0118）。
//
// 口径（只展示已发生的事实，不做权重建模）：
//   - 数据源 = contract A 新接口 /aggregators/reward_streams 返回的窗口内**全部整点快照**；
//   - 每个输出点 = 相邻两个整点快照的计数器差分（矿工实收 / 服务流 / 销毁三股）；
//     miner   = ΔMinerMinted（复用 pkg/londobell 的 minerMinted() 统一 NV29 前后口径：v18 即
//               ΔTotalStoragePowerReward，v19 = ΔTotalMintedReward − ΔTotalBurnMinted − ΔTotalExplicitMinted）；
//     service = ΔTotalExplicitMinted（v18 恒 0）；burn = ΔTotalBurnMinted（v18 恒 0）；
//     total   = miner + service + burn。
//   - 单位 attoFIL（原始计数器值，与统计页其它曲线一致；前端统一用 formatFil ÷1e18 显示）。
//
// nv29_epoch 取本仓既有的 message_detail.UpgradeSolsticeHeight；未排期（UpgradeHeightUnscheduled，
// 主网当前）⇒ 0，前端据此决定不画 NV29 竖线。

// rewardStreamsAgg 「区块奖励流向」用到的聚合器能力（窄接口，便于单测注入假实现）。
type rewardStreamsAgg interface {
	RewardStreams(ctx context.Context, start, end chain.Epoch) ([]*londobell.RewardStream, error)
}

func NewStatisticRewardStreamsBiz(se repository.SyncerGetter, agg rewardStreamsAgg) *StatisticRewardStreamsBiz {
	return &StatisticRewardStreamsBiz{se: se, agg: agg}
}

var _ filscan.StatisticRewardStreams = (*StatisticRewardStreamsBiz)(nil)

type StatisticRewardStreamsBiz struct {
	se  repository.SyncerGetter
	agg rewardStreamsAgg
}

func (s StatisticRewardStreamsBiz) RewardStreams(ctx context.Context, req filscan.RewardStreamsRequest) (resp *filscan.RewardStreamsResponse, err error) {
	current, err := s.se.GetSyncer(ctx, syncer.ChainSyncer)
	if err != nil {
		return
	}
	if current == nil {
		return
	}

	var it interval.Interval
	it, err = interval.ResolveInterval(req.Interval, chain.Epoch(current.Epoch))
	if err != nil {
		return
	}

	// 窗口 [it.Start(), current]（左闭右开：end = current+1，把链头这一行也取进来）。
	snapshots, err := s.agg.RewardStreams(ctx, it.Start(), chain.Epoch(current.Epoch).Next())
	if err != nil {
		return
	}

	resp = &filscan.RewardStreamsResponse{
		Nv29Epoch: nv29EpochOrZero(message_detail.UpgradeSolsticeHeight.Int64()),
		Items:     buildRewardStreamItems(snapshots),
	}
	return
}

// buildRewardStreamItems 把整点快照序列转成「相邻差分」序列。
// 少于两行（或全为空）⇒ 空数组（不是错误，前端画空图）。
// 已按 Epoch 升序排列后逐对求差；计数器理论单调，负增量（快照回滚/数据异常）按 0 计入并打 WARN，
// 保证 total = miner + service + burn 且各分量非负（堆叠图不出现负值）。
func buildRewardStreamItems(snapshots []*londobell.RewardStream) []*filscan.RewardStreamItem {
	items := make([]*filscan.RewardStreamItem, 0)
	ordered := make([]*londobell.RewardStream, 0, len(snapshots))
	for _, s := range snapshots {
		if s != nil {
			ordered = append(ordered, s)
		}
	}
	if len(ordered) < 2 {
		return items
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Epoch < ordered[j].Epoch })

	for i := 1; i < len(ordered); i++ {
		prev, cur := ordered[i-1], ordered[i]
		if cur.Epoch == prev.Epoch {
			continue
		}
		miner := cur.MinerMinted().Sub(prev.MinerMinted())
		service := cur.TotalExplicitMinted.Sub(prev.TotalExplicitMinted)
		burn := cur.TotalBurnMinted.Sub(prev.TotalBurnMinted)

		miner, service, burn = clampFlow(cur.Epoch, miner, service, burn)
		total := miner.Add(service).Add(burn)
		// 单位与统计页其它曲线一致：原始 attoFIL（前端统一用 formatFil ÷1e18 显示）。
		items = append(items, &filscan.RewardStreamItem{
			BlockTime: chain.Epoch(cur.Epoch).Time().Unix(),
			Epoch:     cur.Epoch,
			Miner:     miner,
			Service:   service,
			Burn:      burn,
			Total:     total,
		})
	}
	return items
}

// clampFlow 把负增量归零（数据异常，不静默：打 WARN）。总量在归零后按分量之和重算，保证不变式。
func clampFlow(epoch int64, miner, service, burn decimal.Decimal) (decimal.Decimal, decimal.Decimal, decimal.Decimal) {
	if miner.IsNegative() || service.IsNegative() || burn.IsNegative() {
		log.Warnf("reward_streams: epoch=%d 出现负增量 (miner=%s service=%s burn=%s)，按 0 计入",
			epoch, miner, service, burn)
		miner = nonNegative(miner)
		service = nonNegative(service)
		burn = nonNegative(burn)
	}
	return miner, service, burn
}

func nonNegative(d decimal.Decimal) decimal.Decimal {
	if d.IsNegative() {
		return decimal.Zero
	}
	return d
}

// nv29EpochOrZero 把构建常量映射成对外输出：未排期（UpgradeHeightUnscheduled）或非正值 ⇒ 0。
func nv29EpochOrZero(h int64) int64 {
	if h == int64(buildconstants.UpgradeHeightUnscheduled) || h <= 0 {
		return 0
	}
	return h
}
