package dal

import (
	"strings"
	"testing"
)

// 首页「每赢票奖励」赢票数 SQL 的两个不可退让点（与 dal_biz_miner_reward_range.go 同口径）：
//  1. 区间左闭右开 epoch >= ? and epoch < ?（对齐聚合器管线的 $gte/$lt）；
//  2. 先按 (epoch, miner) 去重再求和 —— chain.miner_win_counts 无唯一约束、写路径是纯 INSERT，
//     重跑同步会产生完全相同的重复行，直接 sum 会把赢票数放大 N 倍（每赢票奖励偏小）。
func TestWinCountRewardSQLSemantics(t *testing.T) {
	sql := normalizeSQL(SQLWinCountRewardStats)
	lower := strings.ToLower(sql)

	if !strings.Contains(lower, "distinct on (epoch, miner)") {
		t.Errorf("必须按 (epoch, miner) 去重（该表无唯一约束、重跑会产生重复行）:\n%s", sql)
	}
	if !strings.Contains(lower, "epoch >= ?") || !strings.Contains(lower, "epoch < ?") {
		t.Errorf("区间必须左闭右开 epoch >= ? and epoch < ?:\n%s", sql)
	}
	if strings.Contains(lower, "epoch <= ?") {
		t.Errorf("不得出现含右端点的 epoch <= ?:\n%s", sql)
	}
	if !strings.Contains(lower, "sum(win_count)") {
		t.Errorf("必须求和 win_count:\n%s", sql)
	}
	if !strings.Contains(lower, "count(distinct epoch)") {
		t.Errorf("覆盖率要看覆盖了多少个高度，必须 count(distinct epoch):\n%s", sql)
	}
	// 去重子查询必须包住 win_count 的来源（否则重复行会被外层 sum 放大）。
	if !strings.Contains(lower, "win_count from chain.miner_win_counts") {
		t.Errorf("win_count 必须取自 DISTINCT ON 子查询:\n%s", sql)
	}
	if strings.Contains(lower, "sum(distinct") {
		t.Error("不得用 sum(distinct win_count)：会把不同 epoch 的相同值也吃掉，口径错")
	}
}
