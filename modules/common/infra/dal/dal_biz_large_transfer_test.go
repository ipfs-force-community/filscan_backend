package dal

import (
	"math"
	"strings"
	"testing"
)

// 这两条 SQL 是大额转账端点「聚合器 vs PG」口径的唯一事实源，改动等于改口径。
// 把三个不可退让的点用断言钉住：
//
//  1. 定序必须是 (epoch desc, cid asc) —— 线上只 $sort:{Epoch:-1}，同高度内行序未定义，
//     PG 侧加 cid 做次级排序是为了让结果确定（**方向不能反**，反了分页整体错位）；
//  2. 绝不能出现 distinct / group by —— 线上 count 管道是 $group{$sum:1} 数命中 trace 的行数、
//     列表管道没有 $group，重数是口径的一部分（去重会比线上少行、分页位移）；
//  3. 计数不按高度区间过滤 —— 请求只带 index/limit，TotalCount 是全表行数。
func TestLargeTransferSQLSemantics(t *testing.T) {
	page := normalizeSQL(SQLLargeTransfersPage)
	if !strings.Contains(page, "from chain.large_transfers") {
		t.Errorf("列表查询必须读 chain.large_transfers:\n%s", page)
	}
	if !strings.Contains(page, "order by epoch desc, cid asc") {
		t.Errorf("列表查询必须按 (epoch desc, cid asc) 定序（次级排序是 PG 相对线上的确定性取舍）:\n%s", page)
	}
	if !strings.Contains(page, "offset ? limit ?") {
		t.Errorf("列表查询必须是 offset/limit 分页（聚合器管线是 $skip/$limit）:\n%s", page)
	}
	for _, banned := range []string{"distinct", "group by", "having"} {
		if strings.Contains(page, banned) {
			t.Errorf("列表查询不得出现 %q（线上按行数返回，去重会让结果比线上少行）:\n%s", banned, page)
		}
	}
	if strings.Contains(page, "where") {
		t.Errorf("列表查询不得带 where：请求只有 index/limit，没有高度区间:\n%s", page)
	}
	// 可空列必须用 coalesce 落到 Go 零值（对齐 mongo null → JSON null → Go "" / 0 的形态）。
	for _, col := range []string{"coalesce(root_cid, '')", "coalesce(from_addr, '')", "coalesce(to_addr, '')", "coalesce(method, '')", "coalesce(depth, 0)", "coalesce(value, 0)"} {
		if !strings.Contains(page, col) {
			t.Errorf("可空列必须 %s（否则扫进 Go string/int64 会报错，或与聚合器零值形态不一致）:\n%s", col, page)
		}
	}

	count := normalizeSQL(SQLCountLargeTransfers)
	if count != "select count(*) as cnt from chain.large_transfers" {
		t.Errorf("计数 SQL 必须是对全表的 count(*)，实际:\n%s", count)
	}
	for _, banned := range []string{"distinct", "where", "group by"} {
		if strings.Contains(count, banned) {
			t.Errorf("计数 SQL 不得出现 %q（口径裁定：全表 count、不去重、不按区间过滤）:\n%s", banned, count)
		}
	}
}

// value 列必须原样扫成 decimal（attoFIL 大整数），**不得**经过 float64：
// 这里用 SQL 文本里没有 ::float8 / double precision 之类写法兜一层，
// 真正的精度验证在 dal 的集成测试与 rewardparity 的单测里（用 1e22 以上的文本）。
func TestLargeTransferValueNotCastedToFloat(t *testing.T) {
	page := normalizeSQL(SQLLargeTransfersPage)
	for _, banned := range []string{"float8", "float4", "double precision", "::numeric::", "cast(", "::text"} {
		if strings.Contains(page, banned) {
			t.Errorf("value 列不得出现 %q（attoFIL 大整数过一次浮点就丢精度；转文本由 decimal 负责）:\n%s", banned, page)
		}
	}
	if !strings.Contains(page, "coalesce(value, 0) as value") {
		t.Errorf("value 列必须原样选出（coalesce(value, 0) as value）:\n%s", page)
	}
}

// index/limit → offset/limit 的折算必须与线上 multi-query/query.go:296-300 一致。
func TestLargeTransferPageWindow(t *testing.T) {
	cases := []struct {
		name                  string
		index, limit          int64
		wantOffset, wantLimit int64
	}{
		{"页码0（线上 skip=0*20=0）", 0, 20, 0, 20},
		{"第1页（线上 skip=1*20=20）", 1, 20, 20, 20},
		{"第7页", 7, 50, 350, 50},
		{"limit=0 且 index>0（线上 limit=0 ⇒ 空页）", 3, 0, 0, 0},
		{"index=limit=0（线上不设分页 ⇒ 全量）", 0, 0, 0, math.MaxInt64},
		{"负数（线上这种输入会让 mongo $skip 报错，这里不做钳制）", -1, 20, -20, 20},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			offset, limit := LargeTransferPageWindow(c.index, c.limit)
			if offset != c.wantOffset || limit != c.wantLimit {
				t.Errorf("LargeTransferPageWindow(%d, %d) = (%d, %d)，期望 (%d, %d)",
					c.index, c.limit, offset, limit, c.wantOffset, c.wantLimit)
			}
		})
	}
}

// 诊断 SQL 只读且只服务排障：不得被读路径引用（这里断言它们的形态，防被误改成计数口径）。
func TestLargeTransferDiagnoseSQL(t *testing.T) {
	dup := normalizeSQL(SQLDuplicateLargeTransfers)
	if !strings.Contains(dup, "group by epoch, cid") || !strings.Contains(dup, "having count(*) > 1") {
		t.Errorf("重复行诊断必须按 (epoch, cid) 分组并只留 count>1 的组:\n%s", dup)
	}
	distinct := normalizeSQL(SQLDistinctCountLargeTransfers)
	if !strings.Contains(distinct, "select distinct epoch, cid from chain.large_transfers") {
		t.Errorf("去重计数是**对照**用（解释 TotalCount 与 distinct 行数的差），形态不对:\n%s", distinct)
	}
}
