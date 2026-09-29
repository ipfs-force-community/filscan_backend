package prodal

import (
	"context"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewRewardDal(db *gorm.DB) *RewardDal {
	return &RewardDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.RewardRepo = (*RewardDal)(nil)

type RewardDal struct {
	*_dal.BaseDal
}

// minerRewardsSQL 取「矿工奖励明细」页一行所需的四类值（同一区间、同一批矿工）：
//
//	a: 区间内（LCRORange，左闭右开）的出块奖励与出块数 —— 逐日口径
//	w: 区间内的赢票数                             —— 逐日口径
//	g: 矿工的累计出块奖励                         —— 累计口径
//
// acc_reward（接口里的 total_reward）取数源说明（2026-09-29 定案，缺陷修复）：
//   - 旧实现从 chain.miner_rewards 的窗口内「最后一个高度」那行读 acc_reward，但这两列
//     （acc_reward / acc_block_count）**从未被任何写入方赋值**（convertor_miner_reward.go 的赋值全部被注释），
//     生产实测近 90 天 1,008,924 行全为 0（非 NULL）⇒ 接口 total_reward 恒为 0；
//   - 现改读 chain.miner_agg_rewards（calc-miner-agg-reward 计算器产出，injector 里注明
//     「计算 Miner 的历史统计值，提供 Pro 使用」）：一矿工一行 = 该矿工的全历史累计出块奖励，
//     近 24h 出过块的矿工覆盖 433/433；
//   - ⚠️ 它是「该矿工上次被那个计算器重算时」的值，不是实时值：实测最近 1 小时出过块的 248 个矿工里
//     207 个与全历史求和精确相等（最差 -0.33%），但停产或历史被回填过的矿工会停在偏小的旧值
//     （近 24h 有出块的 433 个里 190 个偏低，最差 -23.7%）——该偏低要等它再次出块才自愈；
//   - 该表只有 (miner, 累计值) 两个维度、没有 epoch ⇒ 这个「累计」不随请求区间里的“日”变化
//     （代价与备选口径见交付说明）。
const minerRewardsSQL = `
with a as (select miner, sum(reward) as reward, sum(block_count) as block_count
           from chain.miner_rewards
           where epoch < ?
             and epoch >= ?
             and miner in ?
           group by miner),
     w as (select miner, sum(win_count) as win_count
           from chain.miner_win_counts
           where epoch < ?
             and epoch >= ?
             and miner in ?
           group by miner),
     g as (select miner, agg_reward
           from chain.miner_agg_rewards
           where miner in ?)
select coalesce(a.miner, w.miner) as miner,
       a.reward,
       a.block_count,
       w.win_count,
       g.agg_reward as acc_reward
from a
         full join w on a.miner = w.miner
         left join g on g.miner = coalesce(a.miner, w.miner)
order by 1 desc;
`

func (r RewardDal) GetMinerRewards(ctx context.Context, miners []string, epochs chain.LCRORange) (items []prorepo.MinerReward, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(minerRewardsSQL,
		epochs.LtEnd.Int64(),
		epochs.GteBegin.Int64(),
		miners,
		epochs.LtEnd.Int64(),
		epochs.GteBegin.Int64(),
		miners,
		miners,
	).Find(&items).Error
	if err != nil {
		return
	}
	return
}
