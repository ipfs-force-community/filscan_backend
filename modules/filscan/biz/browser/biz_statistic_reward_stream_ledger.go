package browser

import (
	"context"
	"math/big"
	"sort"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件：统计页/首页新接口「服务流账本」（NV29 / FIP-0118，契约 §8.2 E1）。
//
// 与 RewardStreams 的口径区分见 api.StatisticRewardStreamLedger 注释：
// 本接口是 f02 奖励 actor 的**当前状态快照**（待提取欠款 + 当前评估权重日程），
// 不是计数器的窗口差分。数据源为节点侧新端点 POST /adapter/reward_stream_ledger
// （取数走 acl → 节点；本仓不手搓 CBOR、不重算 actor 逻辑）。

// 与链上 reward.Denom 同口径：权重/份额的定点分母。
const rewardDenomUint64 uint64 = 1_000_000_000_000_000_000

var rewardDenomDecimal = decimal.NewFromUint64(rewardDenomUint64)

// rewardStreamLedgerSource 账本取数能力（窄接口，便于单测注入假实现）。
// 由 acl.StatisticAclImpl.GetRewardStreamLedger 满足。
type rewardStreamLedgerSource interface {
	GetRewardStreamLedger(ctx context.Context, epoch *chain.Epoch) (*londobell.RewardStreamLedger, error)
}

func NewStatisticRewardStreamLedgerBiz(se repository.SyncerGetter, ledger rewardStreamLedgerSource) *StatisticRewardStreamLedgerBiz {
	return &StatisticRewardStreamLedgerBiz{se: se, ledger: ledger}
}

var _ filscan.StatisticRewardStreamLedger = (*StatisticRewardStreamLedgerBiz)(nil)

type StatisticRewardStreamLedgerBiz struct {
	se     repository.SyncerGetter
	ledger rewardStreamLedgerSource
}

// RewardStreamLedger 取当前链头的服务流账本并化成对外响应。
//
// 兜底（硬要求：**不得 500**）：
//   - 取当前高度失败 / 账本取数失败 / 节点返回 v18 → 打 WARN + 返回 nv29:false、金额 "0"、recipients 空。
func (s StatisticRewardStreamLedgerBiz) RewardStreamLedger(ctx context.Context, _ filscan.RewardStreamLedgerRequest) (resp *filscan.RewardStreamLedgerResponse, err error) {
	current, epochErr := s.se.GetSyncer(ctx, syncer.ChainSyncer)
	if epochErr != nil || current == nil {
		log.Warnf("reward_stream_ledger: 取当前高度失败 (sync=%v err=%v)，返回 nv29=false 空账本", current != nil, epochErr)
		return emptyRewardStreamLedgerResponse(0), nil
	}
	epoch := chain.Epoch(current.Epoch)

	ledger, ledgerErr := s.ledger.GetRewardStreamLedger(ctx, &epoch)
	if ledgerErr != nil {
		log.Warnf("reward_stream_ledger: epoch=%d 取数失败: %v，返回 nv29=false 空账本", epoch.Int64(), ledgerErr)
		return emptyRewardStreamLedgerResponse(epoch.Int64()), nil
	}
	if ledger == nil || !ledger.Nv29 {
		// 契约 §8.2：v18 / 未激活 NV29 也要打 WARN（该接口按需调用，非热路径；主网 2026-10-19 升 NV29 后自然消失）。
		log.Warnf("reward_stream_ledger: epoch=%d 节点返回 nv29=false（本网未激活 NV29），返回 nv29=false 空账本", epoch.Int64())
	}

	return buildRewardStreamLedgerResponse(ledger, epoch.Int64()), nil
}

// emptyRewardStreamLedgerResponse nv29=false / 取数失败时的兜底响应：金额 "0"、分账比例全 0、受益方空数组。
func emptyRewardStreamLedgerResponse(epoch int64) *filscan.RewardStreamLedgerResponse {
	return &filscan.RewardStreamLedgerResponse{
		Epoch:         epoch,
		Nv29:          false,
		PendingClaim:  decimal.Zero,
		ClaimedPeriod: decimal.Zero,
		CurrentSplit:  &filscan.RewardStreamSplit{Miner: "0.0", Service: "0.0", Burn: "0.0"},
		Recipients:    []*filscan.RewardStreamRecipient{},
	}
}

// buildRewardStreamLedgerResponse 纯函数：把节点账本化成对外响应（v18 / nil ⇒ 回退空账本）。
func buildRewardStreamLedgerResponse(ledger *londobell.RewardStreamLedger, fallbackEpoch int64) *filscan.RewardStreamLedgerResponse {
	epoch := fallbackEpoch
	if ledger != nil && ledger.Epoch > 0 {
		epoch = ledger.Epoch
	}
	if ledger == nil || !ledger.Nv29 {
		return emptyRewardStreamLedgerResponse(epoch)
	}

	denom := ledger.Denom
	if !denom.IsPositive() {
		// 节点未给 denom（老响应）时退回固定 1e18，避免除零。
		denom = rewardDenomDecimal
	}

	var minerWeight, serviceWeight, claimedPeriod decimal.Decimal
	for _, st := range ledger.Streams {
		if st == nil {
			continue
		}
		claimedPeriod = claimedPeriod.Add(st.ClaimedPeriod)
		if st.Implicit {
			minerWeight = minerWeight.Add(st.EvaluatedWeight)
		} else {
			serviceWeight = serviceWeight.Add(st.EvaluatedWeight)
		}
	}

	// burn = Denom − Σ权重（合约要求）。异常（Σ权重 > Denom）时归零并打 WARN，不让堆叠条出现负值。
	burnWeight := denom.Sub(minerWeight).Sub(serviceWeight)
	if burnWeight.IsNegative() {
		log.Warnf("reward_stream_ledger: 权重之和超过 Denom (miner=%s service=%s denom=%s)，burn 按 0 计",
			minerWeight, serviceWeight, denom)
		burnWeight = decimal.Zero
	}

	return &filscan.RewardStreamLedgerResponse{
		Epoch:         epoch,
		Nv29:          true,
		PendingClaim:  ledger.Liability, // 契约：待提取总额 = ledger.Liability()
		ClaimedPeriod: claimedPeriod,
		CurrentSplit: &filscan.RewardStreamSplit{
			Miner:   weightPercent(minerWeight, denom),
			Service: weightPercent(serviceWeight, denom),
			Burn:    weightPercent(burnWeight, denom),
		},
		Recipients: buildRewardStreamRecipients(ledger, denom),
	}
}

// weightPercent 权重占 Denom 的百分比，一位小数（如 "50.0"）。
func weightPercent(weight, denom decimal.Decimal) string {
	if !denom.IsPositive() {
		return "0.0"
	}
	return weight.Mul(decimal.NewFromInt(100)).Div(denom).Round(1).StringFixed(1)
}

// sharePercent 份额占 Denom 的百分比，两位小数（与契约示例 "73.75" 一致）。
func sharePercent(share, denom decimal.Decimal) string {
	if !denom.IsPositive() {
		return "0.00"
	}
	return share.Mul(decimal.NewFromInt(100)).Div(denom).Round(2).StringFixed(2)
}

// rewardStreamRecipientAgg 同一受益地址跨流的合并行（地址 / 份额 / 待付 / 本期已提）。
type rewardStreamRecipientAgg struct {
	share   decimal.Decimal
	pending decimal.Decimal
	claimed decimal.Decimal
	// tombstone 标记该地址在「已移除流」的遗留欠款（ledger.Tombstones）里出现过。它只用于判定
	// removed_stream：只有「只出现在已移除流里（当前份额为 0）」才算是遗留欠款收款人。
	tombstone bool
}

// buildRewardStreamRecipients 汇总各受益地址的份额%、待付与本期已提。
//
// 按地址合并（同一地址出现在多条流时份额、待付、已提相加）。待付口径与 actor 一致：
//   - 显式流行 = 当前期应得(accrued × share / denom) − 当期已提(claimed_period) + 结转未提(payable)；
//   - tombstone 行 = 结转未提(payable)（被移除的流已无份额，share_pct 记 0.00，且无 claimed_period 字段、已提取 0）。
//
// removed_stream = 该地址只出现在已移除流（tombstone）里且当前份额为 0（tombstone && share.IsZero()）：
// 同时在活跃份额表与已移除流里出现的地址不算（当前仍有份额 ⇒ 不是遗留欠款收款人）。
//
// 输出顺序＝「服务受益方排行」顺序：按待付 pending_claim 降序，pending 相等时按地址升序（稳定）；
// 不做截断，全部返回（前端按序取前 N）。
func buildRewardStreamRecipients(ledger *londobell.RewardStreamLedger, denom decimal.Decimal) []*filscan.RewardStreamRecipient {
	agg := make(map[string]*rewardStreamRecipientAgg)
	get := func(addr string) *rewardStreamRecipientAgg {
		if e, ok := agg[addr]; ok {
			return e
		}
		e := &rewardStreamRecipientAgg{}
		agg[addr] = e
		return e
	}

	for _, st := range ledger.Streams {
		if st == nil || st.Implicit {
			continue
		}
		for _, r := range st.Recipients {
			if r == nil {
				continue
			}
			e := get(r.Address)
			e.share = e.share.Add(r.Share)
			current := mulDiv(st.Accrued, r.Share, denom)
			e.pending = e.pending.Add(current).Add(r.Payable).Sub(r.ClaimedPeriod)
			e.claimed = e.claimed.Add(r.ClaimedPeriod)
		}
	}
	for _, t := range ledger.Tombstones {
		if t == nil {
			continue
		}
		for _, r := range t.Recipients {
			if r == nil {
				continue
			}
			e := get(r.Address)
			e.pending = e.pending.Add(r.Payable)
			e.tombstone = true
		}
	}

	addresses := make([]string, 0, len(agg))
	for addr := range agg {
		addresses = append(addresses, addr)
	}
	// 排行表顺序（前端「服务受益方排行」）：按待付 pending_claim 降序；pending 相等时按地址升序（稳定）。
	// 用 decimal 精确比较，**不转 float64**（attoFIL 值远超 float64 有效位数，转 float 会丢精度导致排序错）。
	// 不做 limit / 截断，全部返回。
	sort.SliceStable(addresses, func(i, j int) bool {
		pi, pj := agg[addresses[i]].pending, agg[addresses[j]].pending
		if c := pi.Cmp(pj); c != 0 {
			return c > 0
		}
		return addresses[i] < addresses[j]
	})

	out := make([]*filscan.RewardStreamRecipient, 0, len(addresses))
	for _, addr := range addresses {
		e := agg[addr]
		out = append(out, &filscan.RewardStreamRecipient{
			Address:       chain.SmartAddress(addr).Address(),
			SharePct:      sharePercent(e.share, denom),
			PendingClaim:  e.pending,
			ClaimedPeriod: e.claimed,
			RemovedStream: e.tombstone && e.share.IsZero(),
		})
	}
	return out
}

// mulDiv 整数精确计算 (a×b)/c（a/b/c 均为非负整数值的 decimal），避免 decimal.Div 的精度截断。
func mulDiv(a, b, c decimal.Decimal) decimal.Decimal {
	if a.IsZero() || b.IsZero() || !c.IsPositive() {
		return decimal.Zero
	}
	num := new(big.Int).Mul(a.BigInt(), b.BigInt())
	q := new(big.Int).Quo(num, c.BigInt())
	return decimal.NewFromBigInt(q, 0)
}
