package browser

import (
	"context"
	"math/big"
	"sort"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
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

// rewardStreamRecipientSnapshotSource 「本周期受益方快照」取数能力（窄接口，便于单测注入假实现）。
// 由 dal.RewardStreamRecipientDal 满足（repository.RewardStreamRecipientTask 的只读取子集）。
type rewardStreamRecipientSnapshotSource interface {
	ListRewardStreamRecipientsByEpochRange(ctx context.Context, epochs chain.LCRCRange) ([]*po.RewardStreamRecipientEpoch, error)
}

func NewStatisticRewardStreamLedgerBiz(se repository.SyncerGetter, ledger rewardStreamLedgerSource, recipients rewardStreamRecipientSnapshotSource) *StatisticRewardStreamLedgerBiz {
	return &StatisticRewardStreamLedgerBiz{
		se:            se,
		ledger:        ledger,
		recipients:    recipients,
		solsticeEpoch: nv29EpochOrZero(message_detail.UpgradeSolsticeHeight.Int64()),
	}
}

var _ filscan.StatisticRewardStreamLedger = (*StatisticRewardStreamLedgerBiz)(nil)

type StatisticRewardStreamLedgerBiz struct {
	se     repository.SyncerGetter
	ledger rewardStreamLedgerSource
	// recipients 「本周期受益方快照」只读取（用来补本周期离场者）。表未建 / 未注入时为 nil ⇒ 只按当前链上状态返回。
	recipients rewardStreamRecipientSnapshotSource
	// solsticeEpoch 本网 NV29 激活高度（未排期 ⇒ 0，见 nv29EpochOrZero）；为 0 时不查快照。
	// 构造时取 message_detail.UpgradeSolsticeHeight；单测可覆盖此字段以在默认（mainnet）构建下演练已激活路径。
	solsticeEpoch int64
}

// RewardStreamLedger 取当前链头的服务流账本并化成对外响应。
//
// 兜底（硬要求：**不得 500**）：
//   - 取当前高度失败 / 账本取数失败 / 节点返回 v18 → 打 WARN + 返回 nv29:false、金额 "0"、recipients 空。
//   - 本周期快照取数失败（如表 chain.reward_stream_recipient_epoch 尚未建）→ 打 WARN + 只按当前链上状态返回，
//     接口照常可用（不报错、不回滚）；见 appendDepartedRecipients。
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

	resp = buildRewardStreamLedgerResponse(ledger, epoch.Int64())
	// 本周期离场者：当前高度取同一条链头路径（epoch）。仅在 nv29 已激活且快照可用时补；取数失败自动降级。
	s.appendDepartedRecipients(ctx, resp, ledger, epoch.Int64())
	return resp, nil
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

	denom := rewardLedgerDenom(ledger)

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

// rewardLedgerDenom 账本定点分母；节点未给 denom（老响应）时退回固定 1e18，避免除零。
func rewardLedgerDenom(ledger *londobell.RewardStreamLedger) decimal.Decimal {
	if ledger != nil && ledger.Denom.IsPositive() {
		return ledger.Denom
	}
	return rewardDenomDecimal
}

// rewardStreamRecipientAgg 同一受益地址跨流的合并行（地址 / 份额 / 待付 / 本期已提）。
type rewardStreamRecipientAgg struct {
	share decimal.Decimal
	// accrual 本期应计（Σ accrued×share/denom，仅显式流）；payable 跨周期结转（Σ 链上 Payable，含 tombstone）。
	// pending（总额）在输出时按 accrual + payable − claimed 现算，等于拆分前的口径。
	accrual decimal.Decimal
	payable decimal.Decimal
	pending decimal.Decimal
	claimed decimal.Decimal
	// tombstone 标记该地址在「已移除流」的遗留欠款（ledger.Tombstones）里出现过。它只用于判定
	// removed_stream：只有「只出现在已移除流里（当前份额为 0）」才算是遗留欠款收款人。
	tombstone bool
	// zeroShare 标记该地址在**活跃**流里出现过但份额为 0（链上存在「流还在、份额已被置 0」的收款人：
	// 2026-10-09 cali 实测 t0200442）。判定 zero_share 用，与 tombstone 互斥（tombstone 优先）。
	zeroShare bool
}

// buildRewardStreamRecipients 汇总各受益地址的份额%、待付与本期已提。
//
// 按地址合并（同一地址出现在多条流时份额、待付、已提相加）。待付口径与 actor 一致：
//   - 显式流行 = 当前期应得(accrued × share / denom) − 当期已提(claimed_period) + 结转未提(payable)；
//   - tombstone 行 = 结转未提(payable)（被移除的流已无份额，share_pct 记 0.00，且无 claimed_period 字段、已提取 0）。
//
// removed_stream = 该地址只出现在已移除流（tombstone）里且当前份额为 0（tombstone && share.IsZero()）：
// 同时在活跃份额表与已移除流里出现的地址不算（当前仍有份额 ⇒ 不是遗留欠款收款人）。
// zero_share = 该地址出现在**活跃**份额表里但份额为 0（链上存在「流还在、份额被置 0」的收款人），
// 且没被 tombstone 命中 —— 这类行同样「份额 0% 却有待付」，前端要给它一句说明。
//
// 待付拆两列（2026-10-09 用户裁定）：pending_claim_current（当期＝本期应计−本期已提）与
// pending_claim_carried（跨周期＝链上 Payable，即此前各期已结算未提取的结转）；两列之和恒等于 pending_claim。
//
// 本周期离场者（departed，2026-10-10 追加）：本函数只反映**当前链上状态**——已离场且欠款提完的受益方在链上会
// 彻底消失，单看链头查不到。展示层据此在本函数输出之后补一行（appendDepartedRecipients）：
//   - 周期起点 = nv29Epoch + ((curEpoch−nv29Epoch)/periodLen)*periodLen（整除取整；periodLen =
//     buildconstants.SolsticeEpochsPerQuarter；curEpoch 取链头高度）；
//   - 用快照表 chain.reward_stream_recipient_epoch 取 [周期起点, 当前高度]（左闭右闭）内出现过、
//     但当前链上状态（活跃份额表 + 已移除流遗留欠款）里**没有**的地址，每个地址取本周期内 epoch 最大的那一行；
//   - 该行口径：share_pct="0.00"（当前无份额）、pending_claim_current=0、pending_claim_carried=该行 Payable、
//     pending_claim=carried（保持 pending==current+carried）、departed=true、left_epoch=该行 epoch、
//     last_share_pct=该行 Share 按 sharePercent（同 share_pct 口径）折算；
//   - 排序：活跃行保持原序并一律在前，离场行补在末尾、内部按 pending_claim_carried 降序（相等按地址升序）；
//   - **快照取数失败必须降级**：打 WARN 后只按当前链上状态返回，接口不得 500。
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
			if r.Share.IsZero() {
				// 活跃流里份额为 0：不是「已移除流的遗留欠款」，而是「流还在、这一轮没分到权重」，
				// 前端要另给一句说明（否则和「份额 0% 却有钱」一起看会以为漏标）。
				e.zeroShare = true
			}
			current := mulDiv(st.Accrued, r.Share, denom)
			e.accrual = e.accrual.Add(current)
			e.payable = e.payable.Add(r.Payable)
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
			e.payable = e.payable.Add(r.Payable)
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
		// 当期应收 = 本期应计 − 本期已提（不小于 0）；跨周期应收 = 总额 − 当期应收。
		// 拆分口径（显示约定）：本期提取先冲抵本期应计，超出部分再冲抵跨周期结转 —— 两列之和恒等于 pending_claim。
		current := e.accrual.Sub(e.claimed)
		if current.IsNegative() {
			current = decimal.Zero
		}
		carried := e.pending.Sub(current)
		if carried.IsNegative() {
			carried = decimal.Zero
		}
		out = append(out, &filscan.RewardStreamRecipient{
			Address:             chain.SmartAddress(addr).Address(),
			SharePct:            sharePercent(e.share, denom),
			PendingClaim:        e.pending,
			PendingClaimCurrent: current,
			PendingClaimCarried: carried,
			ClaimedPeriod:       e.claimed,
			RemovedStream:       e.tombstone && e.share.IsZero(),
			ZeroShare:           e.zeroShare && !e.tombstone,
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

// periodStartEpoch 本网 NV29 奖励周期的起点高度（向下对齐到周期边界）：
//
//	start = nv29Epoch + ((curEpoch − nv29Epoch) / periodLen) * periodLen
//
// nv29Epoch 为 NV29 激活高度、periodLen 为周期长度（buildconstants.SolsticeEpochsPerQuarter）。
// 整数整除（全非负）。curEpoch 早于激活高度或 periodLen 非正 ⇒ 退回激活高度本身。
func periodStartEpoch(nv29Epoch, curEpoch, periodLen int64) int64 {
	if periodLen <= 0 || curEpoch <= nv29Epoch {
		return nv29Epoch
	}
	return nv29Epoch + ((curEpoch-nv29Epoch)/periodLen)*periodLen
}

// appendDepartedRecipients 把「本周期内出现过、但当前链上已查不到」的受益方补进排行末尾并标 departed。
//
// 触发条件（任一不满足即不查快照，保持现有行为）：resp 有效、账本 nv29=true、本网已排期（solsticeEpoch>0）、
// 快照仓储已注入、当前高度不早于激活高度、周期长度为正。
//
// 降级（硬要求：**不得 500**）：快照查询报错（如表 chain.reward_stream_recipient_epoch 尚未建）时打 WARN，
// 按「只有当前链上状态」返回，接口照常可用。
func (s StatisticRewardStreamLedgerBiz) appendDepartedRecipients(ctx context.Context, resp *filscan.RewardStreamLedgerResponse, ledger *londobell.RewardStreamLedger, curEpoch int64) {
	if resp == nil || !resp.Nv29 || s.solsticeEpoch <= 0 || s.recipients == nil {
		return
	}
	if curEpoch < s.solsticeEpoch {
		return
	}
	periodLen := int64(buildconstants.SolsticeEpochsPerQuarter)
	if periodLen <= 0 {
		return
	}
	start := periodStartEpoch(s.solsticeEpoch, curEpoch, periodLen)
	rows, err := s.recipients.ListRewardStreamRecipientsByEpochRange(ctx, chain.NewLCRCRange(chain.Epoch(start), chain.Epoch(curEpoch)))
	if err != nil {
		log.Warnf("reward_stream_ledger: 取本周期(%d..%d)受益方快照失败: %v，按只有当前链上状态返回", start, curEpoch, err)
		return
	}
	if len(rows) == 0 {
		return
	}
	resp.Recipients = mergeDepartedRecipients(resp.Recipients, rows, rewardLedgerDenom(ledger))
}

// mergeDepartedRecipients 把快照行合并到活跃排行：仅补「快照里出现过、当前排行（含已移除流遗留行）里没有」的地址，
// 每个地址取本周期内 epoch 最大的那一行。活跃行保持原序并一律在前；离场行补在末尾，内部按跨周期结转(carried)
// 降序（相等按地址升序）。快照地址与排行地址均归一化（SmartAddress）后比较，避免前缀差异造成漏配/重复。
func mergeDepartedRecipients(active []*filscan.RewardStreamRecipient, rows []*po.RewardStreamRecipientEpoch, denom decimal.Decimal) []*filscan.RewardStreamRecipient {
	present := make(map[string]struct{}, len(active))
	for _, r := range active {
		if r != nil {
			present[r.Address] = struct{}{}
		}
	}

	latest := make(map[string]*po.RewardStreamRecipientEpoch)
	for _, row := range rows {
		if row == nil {
			continue
		}
		addr := chain.SmartAddress(row.Address).Address()
		if _, ok := present[addr]; ok {
			continue // 当前链上仍在（含 tombstone 遗留行）⇒ 不算离场，不补
		}
		if cur, ok := latest[addr]; !ok || row.Epoch > cur.Epoch {
			latest[addr] = row
		}
	}
	if len(latest) == 0 {
		return active
	}

	type departedRecipient struct {
		addr      string
		leftEpoch int64
		carried   decimal.Decimal
		claimed   decimal.Decimal
		lastShare string
	}
	departed := make([]departedRecipient, 0, len(latest))
	for addr, row := range latest {
		carried := nonNegative(row.Payable)
		departed = append(departed, departedRecipient{
			addr:      addr,
			leftEpoch: row.Epoch,
			carried:   carried,
			claimed:   row.ClaimedPeriod,
			lastShare: sharePercent(row.Share, denom),
		})
	}
	// 离场行排序：跨周期结转(carried)降序，相等按地址升序（与活跃行的 pending 降序+地址升序同风格）。
	sort.SliceStable(departed, func(i, j int) bool {
		if c := departed[i].carried.Cmp(departed[j].carried); c != 0 {
			return c > 0
		}
		return departed[i].addr < departed[j].addr
	})

	out := make([]*filscan.RewardStreamRecipient, 0, len(active)+len(departed))
	out = append(out, active...)
	for _, d := range departed {
		// 离场行：当前无份额 ⇒ share_pct 记 0.00、当期应收 0；金额全部是此前结转（carried），
		// 保持不变式 pending_claim == pending_claim_current + pending_claim_carried。
		out = append(out, &filscan.RewardStreamRecipient{
			Address:             d.addr,
			SharePct:            "0.00",
			PendingClaim:        d.carried,
			PendingClaimCurrent: decimal.Zero,
			PendingClaimCarried: d.carried,
			ClaimedPeriod:       d.claimed,
			Departed:            true,
			LeftEpoch:           d.leftEpoch,
			LastSharePct:        d.lastShare,
		})
	}
	return out
}
