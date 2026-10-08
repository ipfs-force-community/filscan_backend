// Package calc_reward_stream_recipient_task 采集「f02 奖励流（NV29 / FIP-0118）受益方」的两张派生表：
//
//  1. chain.reward_stream_recipient_epoch  —— 按高度快照（唯一键 (epoch, address)），
//     服务「当前链上周期内出现过的受益方」，会被框架 HistoryClear 按高度修剪；
//  2. chain.reward_stream_recipient_period —— 按周期归集（唯一键 (address, period_start_epoch)），
//     服务「累计已收」，**只增不减、跨周期保留**，HistoryClear 不清理（见下）。
//
// 目标表 / 为什么是两张：
//   - 快照表：链上只保存当前状态，受益方被移出且欠款提完后会彻底消失；产品要标「本期已离场」，
//     必须在每个高度把当时在场的受益方快照下来（DDL 见 migration/37.reward_stream_recipient_epoch.sql）。
//   - 归集表：链上 ClaimedPeriod 到下一周期就归零，累计值链上并不存在；快照表又会随 HistoryClear
//     被修剪，不能跨周期。故另立按周期归集的表，由上层按地址 SUM 出「累计已收」
//     （DDL 见 migration/38.reward_stream_recipient_period.sql）。
//
// 对应生产装配：chain 同步器的 WithCalculators 列表
// （injector/syncer_manager.go，与 calc-miner-acc-reward-task 同一列表）。
// 它是**计算器**（Calculator，对高度连续性有依赖），但每个高度都写（只受 ctx.Empty() 限制）。
//
// 触发条件：每个高度执行；ctx.Empty()（该高度在聚合器侧为空）时直接返回、不请求节点、不写。
//
// 输入/取数：**不用聚合器、不读 traces、不用 Datamap**，只用适配器：
//
//	ctx.Adapter().RewardStreamLedger(ctx, &epoch) —— 节点端点 POST /adapter/reward_stream_ledger
//	（实现见 pkg/londobell/impl/adapter_impl.go，接口 pkg/londobell/adapter.go）。
//	节点返回 ledger.Nv29=false（本网尚未激活 NV29）时属**正常响应**，本计算器整高度不写。
//	**同一份 ledger 同时喂给两张表，绝不额外请求节点。**
//
// 快照表落行规则（buildRecipientEpochRows）：
//  1. 每条**显式流**（Implicit=false）的每个受益方：share / payable / claimed_period 取链上值；
//  2. ledger.tombstones 里的每个遗留受益方：share 记 0、payable 取链上值、claimed_period 记 0、
//     tombstone=true（不能再靠 share=0 反推「离场」——链上存在「流还在、份额被置 0」的活跃收款人）；
//  3. 同一地址出现在多条流里时**按地址合并**（唯一键 (epoch, address) 不允许同高度同地址多行）：
//     share / payable / claimed_period 累加，tombstone 取或。合并口径与展示层
//     modules/filscan/biz/browser/biz_statistic_reward_stream_ledger.go 的 buildRewardStreamRecipients 一致。
//
// 归集表落行规则（buildRecipientPeriodRows）：
//  1. **只有显式流**的受益方计入：每条显式流的每个受益方按 (address, 周期起点) 累加链上 claimed_period；
//  2. tombstone 的遗留受益方 claimed_period 记 0，**不进归集表**（不影响累计）；
//  3. 周期起点 = nv29Epoch + ((epoch-nv29Epoch)/periodLen)*periodLen（整除，向下取整）。
//     periodLen 由构造器**注入**（生产取 buildconstants.SolsticeEpochsPerQuarter；
//     主网 262974、calibnet 2880），便于单测用两个网络的值各验一例。
//
// 幂等性 / 重跑纪律：
//   - 快照表：唯一键 (epoch, address) + Save 侧 ON CONFLICT DO UPDATE ⇒ 同高度重跑后与只跑一次相同；
//   - 归集表：唯一键 (address, period_start_epoch) + Upsert 侧 ON CONFLICT DO UPDATE，且
//     claimed_in_period 取 GREATEST、first_epoch 取 LEAST、last_epoch 取 GREATEST ⇒
//     同高度重跑、同周期跨高度重复观测都会得到与只跑一次相同的结果，且不把大值降回小值。
//
// 回滚 / 历史清理（框架 base 接口）：
//   - RollBack(gteEpoch)     => 快照表 delete where epoch >= gteEpoch；
//     归集表 delete where last_epoch >= gteEpoch。
//     注意：跨周期回滚会丢掉被删行所在周期**已闭合**的归集值，
//     只能靠「回滚后重新同步（回放）该区间」重建——这是刻意的取舍：
//     若不删，回滚后 last_epoch 指向已不存在的链，累计值会失真。
//   - HistoryClear(lteEpoch) => 快照表 delete where epoch <= lteEpoch；
//     归集表**不清理**（累计必须跨周期保留，删了就永久丢失），仅记 debug 日志。
package calc_reward_stream_recipient_task

import (
	"context"

	logging "github.com/gozelle/logger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"

	"github.com/filecoin-project/lotus/build/buildconstants"
	"github.com/shopspring/decimal"
)

var logger = logging.NewLogger("calc-reward-stream-recipient-task")

// NewCalcRewardStreamRecipientTask 生产装配：周期长度取本网编译期常量
// （主网 buildconstants.SolsticeEpochsPerQuarter=262974；calibnet build tag 下为 EpochsInDay=2880），
// NV29 起点取升级高度 message_detail.UpgradeSolsticeHeight。
func NewCalcRewardStreamRecipientTask(repo repository.RewardStreamRecipientTask, periodRepo repository.RewardStreamRecipientPeriodTask) *CalcRewardStreamRecipientTask {
	return &CalcRewardStreamRecipientTask{
		repo:       repo,
		periodRepo: periodRepo,
		nv29Epoch:  message_detail.UpgradeSolsticeHeight.Int64(),
		periodLen:  int64(buildconstants.SolsticeEpochsPerQuarter),
	}
}

// NewCalcRewardStreamRecipientTaskWithPeriod 供单测注入周期参数（便于用两个网络的
// nv29Epoch / periodLen 各验一例）。生产请用 NewCalcRewardStreamRecipientTask。
func NewCalcRewardStreamRecipientTaskWithPeriod(repo repository.RewardStreamRecipientTask, periodRepo repository.RewardStreamRecipientPeriodTask, nv29Epoch, periodLen int64) *CalcRewardStreamRecipientTask {
	return &CalcRewardStreamRecipientTask{repo: repo, periodRepo: periodRepo, nv29Epoch: nv29Epoch, periodLen: periodLen}
}

var _ syncer.Calculator = (*CalcRewardStreamRecipientTask)(nil)

type CalcRewardStreamRecipientTask struct {
	repo       repository.RewardStreamRecipientTask
	periodRepo repository.RewardStreamRecipientPeriodTask
	nv29Epoch  int64 // NV29 激活高度（周期起点公式的基准）
	periodLen  int64 // 周期长度（可注入，见 WithPeriod 构造器）
}

func (c CalcRewardStreamRecipientTask) Name() string {
	return "calc-reward-stream-recipient-task"
}

// RollBack 链回滚：
//   - 快照表清除所有 epoch >= gteEpoch 的行（重跑会按 idempotent upsert 重写）；
//   - 归集表删除 last_epoch >= gteEpoch 的行。
//
// 跨周期回滚会丢掉被删行所在周期**已闭合**的归集值（链上已归零、无法直接重建），
// 只能靠回滚后重新同步（回放）该区间重建——这是刻意的取舍。
func (c CalcRewardStreamRecipientTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	if err = c.repo.DeleteRewardStreamRecipientsGteEpoch(ctx, gteEpoch); err != nil {
		return
	}
	return c.periodRepo.DeleteRewardStreamRecipientPeriodsGteEpoch(ctx, gteEpoch)
}

// HistoryClear 只清理快照表历史行；**归集表不清理**。
//
// 理由：归集表是「累计已收」的唯一来源，必须跨周期保留——删掉任一已闭合周期的行就永久丢失
// 该周期已收（链上 ClaimedPeriod 已归零，无法重建）。框架要求实现本方法，但这里只记 debug 日志。
func (c CalcRewardStreamRecipientTask) HistoryClear(ctx context.Context, lteEpoch chain.Epoch) (err error) {
	logger.Debugf("HistoryClear: 保留 chain.reward_stream_recipient_period（累计已收只增不减），仅清理快照表 <= %d", lteEpoch.Int64())
	return c.repo.DeleteRewardStreamRecipientsLteEpoch(ctx, lteEpoch)
}

// Calc 取该高度的奖励流账本，用同一份 ledger 落两类派生行：
// 快照表（当前周期留痕）与归集表（累计已收）。
func (c CalcRewardStreamRecipientTask) Calc(ctx *syncer.Context) (err error) {
	if ctx.Empty() {
		// 该高度在聚合器侧为空（无区块），链上状态未推进，无需落任何行。
		return
	}

	epoch := ctx.Epoch()
	ledger, err := ctx.Adapter().RewardStreamLedger(ctx.Context(), &epoch)
	if err != nil {
		return
	}
	if ledger == nil || !ledger.Nv29 {
		// 契约 §8.1：v18（本网未激活 NV29）返回 nv29=false、streams=[] —— 正常响应，整高度不写。
		return
	}

	// 快照表：当前周期内出现过的受益方（可被 HistoryClear 按高度修剪）。
	items := buildRecipientEpochRows(epoch.Int64(), ledger)
	if len(items) > 0 {
		if err = c.repo.SaveRewardStreamRecipients(ctx.Context(), items); err != nil {
			return
		}
	}

	// 归集表：累计已收（跨周期保留、只增不减）。同一份 ledger，不再请求节点。
	periodItems := buildRecipientPeriodRows(epoch.Int64(), c.nv29Epoch, c.periodLen, ledger)
	if len(periodItems) > 0 {
		if err = c.periodRepo.UpsertRewardStreamRecipientPeriods(ctx.Context(), periodItems); err != nil {
			return
		}
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

// buildRecipientPeriodRows 把某高度账本化成**归集行**：只取**显式流**的受益方，
// 按 (address, 周期起点) 累加链上 claimed_period（同高度同地址出现在多条显式流里时累加）。
// tombstone 的遗留受益方 claimed_period 记 0，不进本表。
//
// first_epoch / last_epoch 记当前高度（同一 (address, period) 跨高度时由仓储侧 LEAST/GREATEST 合并）。
// 纯函数（不碰 ctx / db），便于单测直接覆盖周期起点与合并口径。输出按地址首次出现顺序，保证确定性。
func buildRecipientPeriodRows(epoch, nv29Epoch, periodLen int64, ledger *londobell.RewardStreamLedger) (rows []*po.RewardStreamRecipientPeriod) {
	periodStart := periodStartForEpoch(epoch, nv29Epoch, periodLen)

	agg := make(map[string]*po.RewardStreamRecipientPeriod)
	order := make([]string, 0)
	for _, st := range ledger.Streams {
		if st == nil || st.Implicit {
			// 隐式流是矿工共识流，且 tombstone 不计入累计 —— 只归集显式流。
			continue
		}
		for _, r := range st.Recipients {
			if r == nil {
				continue
			}
			e, ok := agg[r.Address]
			if !ok {
				e = &po.RewardStreamRecipientPeriod{
					Address:          r.Address,
					PeriodStartEpoch: periodStart,
					ClaimedInPeriod:  decimal.Zero,
					FirstEpoch:       epoch,
					LastEpoch:        epoch,
				}
				agg[r.Address] = e
				order = append(order, r.Address)
			}
			e.ClaimedInPeriod = e.ClaimedInPeriod.Add(r.ClaimedPeriod)
		}
	}

	rows = make([]*po.RewardStreamRecipientPeriod, 0, len(order))
	for _, addr := range order {
		rows = append(rows, agg[addr])
	}
	return
}

// periodStartForEpoch 计算某高度所在周期的起点高度（闭周期左端）：
//
//	periodStart = nv29Epoch + ((epoch - nv29Epoch) / periodLen) * periodLen   （整除，向下取整）
//
// 调用方保证 epoch >= nv29Epoch（NV29 激活后才会有奖励流）；periodLen <= 0 时退化为「高度即周期」
// （此时每高度自成周期，仅用于防御非法配置，正常装配不会出现）。
func periodStartForEpoch(epoch, nv29Epoch, periodLen int64) int64 {
	if periodLen <= 0 {
		return epoch
	}
	return nv29Epoch + ((epoch-nv29Epoch)/periodLen)*periodLen
}
