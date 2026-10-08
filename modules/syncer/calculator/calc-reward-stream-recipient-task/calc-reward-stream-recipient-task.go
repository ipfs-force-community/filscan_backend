// Package calc_reward_stream_recipient_task 采集「f02 奖励流（NV29 / FIP-0118）受益方按高度快照」。
//
// 目标表：chain.reward_stream_recipient_epoch（唯一键 (epoch, address)，
// DDL 见 migration/37.reward_stream_recipient_epoch.sql）——每个高度、每个受益地址一行。
//
// 对应生产装配：chain 同步器的 WithCalculators 列表
// （injector/syncer_manager.go，与 calc-miner-acc-reward-task 同一列表）。
// 它是**计算器**（Calculator，对高度连续性有依赖），但与其它计算器不同：每个高度都写
// （只受 ctx.Empty() 限制）。
//
// 触发条件：每个高度执行；ctx.Empty()（该高度在聚合器侧为空）时直接返回、不请求节点、不写。
//
// 输入/取数：**不用聚合器、不读 traces、不用 Datamap**，只用适配器：
//
//	ctx.Adapter().RewardStreamLedger(ctx, &epoch) —— 节点端点 POST /adapter/reward_stream_ledger
//	（实现见 pkg/londobell/impl/adapter_impl.go，接口 pkg/londobell/adapter.go）。
//	节点返回 ledger.Nv29=false（本网尚未激活 NV29）时属**正常响应**，本计算器整高度不写。
//
// 落行规则：
//  1. 每条**显式流**（Implicit=false）的每个受益方：share / payable / claimed_period 取链上值；
//  2. ledger.tombstones 里的每个遗留受益方：share 记 0、payable 取链上值、claimed_period 记 0、
//     tombstone=true（不能再靠 share=0 反推「离场」——链上存在「流还在、份额被置 0」的活跃收款人）；
//  3. 同一地址出现在多条流里时**按地址合并**（唯一键 (epoch, address) 不允许同高度同地址多行）：
//     share / payable / claimed_period 累加，tombstone 取或。合并口径与展示层
//     modules/filscan/biz/browser/biz_statistic_reward_stream_ledger.go 的 buildRewardStreamRecipients 一致。
//
// 幂等性 / 重跑纪律：chain.reward_stream_recipient_epoch 的唯一键 (epoch, address) +
// Save 侧 ON CONFLICT DO UPDATE ⇒ 同一高度重跑（重试、回放）后表内容与只跑一次完全相同，
// **不会**产生重复行（区别于没有唯一键的 chain.miner_reward_stats）。
//
// 回滚 / 历史清理（框架 base 接口）：
//   - RollBack(gteEpoch)     => delete where epoch >= gteEpoch（链回滚）；
//   - HistoryClear(lteEpoch) => delete where epoch <= lteEpoch（历史清理）。
package calc_reward_stream_recipient_task

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"

	"github.com/shopspring/decimal"
)

func NewCalcRewardStreamRecipientTask(repo repository.RewardStreamRecipientTask) *CalcRewardStreamRecipientTask {
	return &CalcRewardStreamRecipientTask{repo: repo}
}

var _ syncer.Calculator = (*CalcRewardStreamRecipientTask)(nil)

type CalcRewardStreamRecipientTask struct {
	repo repository.RewardStreamRecipientTask
}

func (c CalcRewardStreamRecipientTask) Name() string {
	return "calc-reward-stream-recipient-task"
}

// RollBack 链回滚：清除所有 >= gteEpoch 的快照行（重跑会按 idempotent upsert 重写）。
func (c CalcRewardStreamRecipientTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	return c.repo.DeleteRewardStreamRecipientsGteEpoch(ctx, gteEpoch)
}

// HistoryClear 统一清除历史数据：清除所有 <= lteEpoch 的快照行。
func (c CalcRewardStreamRecipientTask) HistoryClear(ctx context.Context, lteEpoch chain.Epoch) (err error) {
	return c.repo.DeleteRewardStreamRecipientsLteEpoch(ctx, lteEpoch)
}

// Calc 取该高度的奖励流账本并按受益方落快照行。
func (c CalcRewardStreamRecipientTask) Calc(ctx *syncer.Context) (err error) {
	if ctx.Empty() {
		// 该高度在聚合器侧为空（无区块），链上状态未推进，无需快照。
		return
	}

	epoch := ctx.Epoch()
	ledger, err := ctx.Adapter().RewardStreamLedger(ctx.Context(), &epoch)
	if err != nil {
		return
	}
	if ledger == nil || !ledger.Nv29 {
		// 契约 §8.1：v18（本网未激活 NV29）返回 nv29=false、streams=[] —— 正常响应，不写。
		return
	}

	items := buildRecipientEpochRows(epoch.Int64(), ledger)
	if len(items) == 0 {
		return
	}

	err = c.repo.SaveRewardStreamRecipients(ctx.Context(), items)
	if err != nil {
		return
	}

	return
}

// buildRecipientEpochRows 把某高度账本化成快照行：按受益地址合并（唯一键 (epoch, address)），
// 显式流的 share/payable/claimed_period 累加，tombstone 的 payable 累加并置 tombstone=true。
//
// 纯函数（不碰 ctx / db），便于单测直接覆盖合并口径。输出按地址首次出现顺序，保证确定性。
func buildRecipientEpochRows(epoch int64, ledger *londobell.RewardStreamLedger) (rows []*po.RewardStreamRecipientEpoch) {
	agg := make(map[string]*po.RewardStreamRecipientEpoch)
	order := make([]string, 0)
	get := func(addr string) *po.RewardStreamRecipientEpoch {
		if r, ok := agg[addr]; ok {
			return r
		}
		r := &po.RewardStreamRecipientEpoch{
			Epoch:         epoch,
			Address:       addr,
			Share:         decimal.Zero,
			Payable:       decimal.Zero,
			ClaimedPeriod: decimal.Zero,
		}
		agg[addr] = r
		order = append(order, addr)
		return r
	}

	for _, st := range ledger.Streams {
		if st == nil || st.Implicit {
			// 隐式流是矿工共识流（无 shares），不落快照。
			continue
		}
		for _, r := range st.Recipients {
			if r == nil {
				continue
			}
			e := get(r.Address)
			e.Share = e.Share.Add(r.Share)
			e.Payable = e.Payable.Add(r.Payable)
			e.ClaimedPeriod = e.ClaimedPeriod.Add(r.ClaimedPeriod)
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
			// 已移除流无份额 / 无 claimed_period：share 保持 0，只累加结转欠款。
			e.Payable = e.Payable.Add(r.Payable)
			e.Tombstone = true
		}
	}

	rows = make([]*po.RewardStreamRecipientEpoch, 0, len(order))
	for _, addr := range order {
		rows = append(rows, agg[addr])
	}
	return
}
