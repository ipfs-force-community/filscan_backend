package dal

import (
	"strings"
	"testing"
)

// 这三条 SQL 是「聚合器 vs PG」口径的唯一事实源，改动等于改口径，用断言把两个不可退让的点钉住：
//
//  1. 区间必须左闭右开（epoch < ?）—— 聚合器管线是 Epoch:{$gte,$lt}
//  2. 必须先按 (epoch, miner) 去重再聚合 —— 写路径是纯 INSERT，重复行会把金额/赢票数放大
func TestRewardRangeSQLSemantics(t *testing.T) {
	cases := []struct {
		name     string
		sql      string
		hasMiner bool
		needsSum bool
	}{
		{"MinerBlockRewardRange", SQLMinerBlockRewardRange, true, false},
		{"MinersBlockRewardRange", SQLMinersBlockRewardRange, false, false},
		{"MinerWinCountsRange", SQLMinerWinCountsRange, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lower := strings.ToLower(c.sql)
			if !strings.Contains(lower, "distinct on (epoch, miner)") {
				t.Errorf("%s 必须按 (epoch, miner) 去重（写路径是纯 INSERT，重跑会产生重复行）", c.name)
			}
			if !strings.Contains(lower, "epoch >= ?") || !strings.Contains(lower, "epoch < ?") {
				t.Errorf("%s 区间必须左闭右开 epoch >= ? and epoch < ?（聚合器管线是 $gte/$lt）", c.name)
			}
			if strings.Contains(lower, "epoch <= ?") {
				t.Errorf("%s 不得出现含右端点的 epoch <= ?", c.name)
			}
			if c.hasMiner && !strings.Contains(lower, "miner = ?") {
				t.Errorf("%s 必须按 miner 过滤", c.name)
			}
			if c.needsSum && !strings.Contains(lower, "sum(win_count)") {
				t.Errorf("%s 必须按 miner 求和 win_count", c.name)
			}
			if c.needsSum && !strings.Contains(lower, "sum(gas_reward)") {
				t.Errorf("%s 必须按 miner 求和 gas_reward（migration/36 的 TotalGasReward 来源）", c.name)
			}
			if c.needsSum && !strings.Contains(lower, "count(gas_reward)") {
				t.Errorf("%s 必须带 count(gas_reward) 回填进度探针（否则读路径分不清 NULL 与 0）", c.name)
			}
		})
	}
}

// win_counts 求和必须发生在去重之后：外层 sum 的数据来源必须是带 DISTINCT ON 的子查询，
// 否则重复行会被直接放大（同一高度重跑同步 → 赢票数 ×N）。
func TestMinerWinCountsDedupBeforeSum(t *testing.T) {
	sql := normalizeSQL(SQLMinerWinCountsRange)
	want := "select miner, sum(win_count) as win_count, sum(gas_reward) as gas_reward, count(*) as total_rows, count(gas_reward) as gas_reward_rows from (select distinct on (epoch, miner)"
	if !strings.Contains(sql, want) {
		t.Errorf("必须是「外层 sum、数据来源是 DISTINCT ON 子查询」，实际 SQL 结构不符:\n%s", sql)
	}
	if !strings.Contains(sql, "gas_reward from chain.miner_win_counts") {
		t.Errorf("gas_reward 必须在 DISTINCT ON 子查询内（否则重复行会把 gas 放大 N 倍）:\n%s", sql)
	}
	if strings.Contains(sql, "sum(distinct") {
		t.Error("不得用 sum(distinct win_count)：它会把不同 epoch 的相同值也吃掉，口径错")
	}
}

// normalizeSQL 折叠空白，便于对 SQL 结构做断言（不受换行/缩进影响）。
func normalizeSQL(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
