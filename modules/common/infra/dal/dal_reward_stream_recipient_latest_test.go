package dal

import (
	"strings"
	"testing"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
)

// 「离场者」快照来源的 SQL 口径守卫：每个地址取**全局** epoch 最大的一行（不是「本周期内」最大）。
//
// 为什么单列这一条：2026-10-13 cali 实测 t0200206 只在快照表里有行、且行都在更早周期
// （归集表完全无它，tombstone 遗留欠款收款人按设计不进退集表）——只取本周期范围会把它漏掉，
// 页面就少一行。这条钉住 SQL 形态（DISTINCT ON(address) + 无周期下界），
// biz 层用例（TestRewardStreamLedgerDepartedFromSnapshotEarlierPeriod / ...PicksLatestEpoch）钉住展示口径。
//
// 变异口径：把 `where epoch <= ?` 改成 `where epoch >= ? and epoch <= ?`（退化成「本周期内最大」）
// 会让本用例在「不得有 epoch 下界」处失败。
func TestLatestRewardStreamRecipientEpochPerAddressSQL(t *testing.T) {
	sql := strings.ToLower(normalizeSQL(latestRewardStreamRecipientEpochPerAddressSQL()))

	if !strings.Contains(sql, "distinct on (address)") {
		t.Fatalf("必须每个地址只取一行（DISTINCT ON (address)），实际:\n%s", sql)
	}
	if !strings.Contains(sql, "order by address asc, epoch desc") {
		t.Fatalf("必须按 address asc, epoch desc 定序（每地址取 epoch 最大那行），实际:\n%s", sql)
	}
	if !strings.Contains(sql, "epoch <= ?") {
		t.Fatalf("必须有全局上界 epoch <= ?（限于链头高度以内），实际:\n%s", sql)
	}
	// 关键：不得出现周期下界（epoch >= ?）——否则退化成「本周期内最大」，漏掉更早周期的地址。
	if strings.Contains(sql, "epoch >= ?") {
		t.Fatalf("不得有 epoch 下界（那会退化成「本周期内最大」，漏掉更早周期离场的地址），实际:\n%s", sql)
	}
	// 表名必须来自 PO.TableName()（不硬编码，改名后不会指向错表）。
	if !strings.Contains(sql, po.RewardStreamRecipientEpoch{}.TableName()) {
		t.Fatalf("表名必须来自 PO.TableName()=%s，实际:\n%s", po.RewardStreamRecipientEpoch{}.TableName(), sql)
	}
}
