package prodal

import (
	"strings"
	"testing"
)

// 「pro 后台-矿工奖励明细」total_reward 恒 0 的缺陷回归（缺陷二）：
//
//	旧实现从 chain.miner_rewards 里挑「窗口内最后一个高度」的那行，读它的 acc_reward；
//	而 chain.miner_rewards 的 acc_reward / acc_block_count / prev_reward_ref 这三列**从未被写入**
//	（modules/common/infra/convertor/convertor_miner_reward.go 的赋值全部被注释），
//	生产实测近 90 天 1,008,924 行全为 0（非 NULL）⇒ 接口 total_reward 恒 0。
//
// 现在改读 chain.miner_agg_rewards（calc-miner-agg-reward 产出，注释写明「提供 Pro 使用」）。
// 单测在此锁住 SQL 形状，防止退回死列（数值层面的核实见本次交付的生产只读抽样）。
func TestMinerRewardsSQLReadsMaintainedAggRewardSource(t *testing.T) {
	sql := strings.Join(strings.Fields(minerRewardsSQL), " ")

	// 累计奖励的新取数源
	if !strings.Contains(sql, "from chain.miner_agg_rewards") {
		t.Fatalf("累计奖励必须取自 chain.miner_agg_rewards（已在维护的活表），实际 SQL：%s", sql)
	}
	// acc_reward 只允许出现一次（= 聚合表列 agg_reward 的别名），
	// 即不再从 chain.miner_rewards 读那个从未被写入的同名列
	if n := strings.Count(sql, "acc_reward"); n != 1 {
		t.Fatalf("SQL 里 acc_reward 应只出现 1 次（聚合表别名），实际 %d 次 ⇒ 疑似又读了 miner_rewards 的死列：%s", n, sql)
	}
	// 旧实现的三处特征：窗口内取最后一行 + 那两个从未写入的列
	for _, legacy := range []string{"rank() over", "acc_block_count", "prev_reward_ref"} {
		if strings.Contains(sql, legacy) {
			t.Fatalf("SQL 里出现旧实现的痕迹 %q（死列/取最后一行的写法），实际 SQL：%s", legacy, sql)
		}
	}

	// 逐日口径（出块奖励、出块数、赢票数）必须保留
	if !strings.Contains(sql, "from chain.miner_rewards") || !strings.Contains(sql, "from chain.miner_win_counts") {
		t.Fatalf("逐日明细的取数（chain.miner_rewards / chain.miner_win_counts）被破坏：%s", sql)
	}
	// 绑定参数个数：miner_rewards 3 + miner_win_counts 3 + miner_agg_rewards 1
	if n := strings.Count(sql, "?"); n != 7 {
		t.Fatalf("绑定参数个数应为 7，实际 %d（区间/矿工过滤被改动？）：%s", n, sql)
	}
}
