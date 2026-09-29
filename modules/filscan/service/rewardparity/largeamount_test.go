package rewardparity

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 1e22 attoFIL（FIL>=10000 的门槛值）与 38 位极值：两侧都必须原样（decimal 文本全等）。
const (
	laThreshold = "10000000000000000000000"
	laMaxValue  = "99999999999999999999999999999999999999"
)

var laParams = LargeAmountParams{Index: 0, Limit: 2, Offset: 0, PageLimit: 2}

func laAggRow(epoch int64, cid, rootCid, from, to, value, method string, depth int64) *londobell.ActorMessages {
	return &londobell.ActorMessages{
		Cid: cid, RootCid: rootCid, Epoch: epoch,
		From: chain.SmartAddress(from), To: chain.SmartAddress(to),
		Value: decimal.RequireFromString(value), Method: method, Depth: depth,
	}
}

func laPgRow(epoch int64, cid, rootCid, from, to, value, method string, depth int64) *bo.LargeTransferRow {
	return &bo.LargeTransferRow{
		Epoch: epoch, Cid: cid, RootCid: rootCid,
		FromAddr: from, ToAddr: to, Value: decimal.RequireFromString(value), Method: method, Depth: depth,
	}
}

// 两侧逐行一致 → PASS（含同键重复行：跨库边界重复是口径的一部分，重数必须一致）。
func TestCompareLargeAmountParity(t *testing.T) {
	const (
		cid1 = "bafy2bzaced-1"
		from = "f410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa"
		to   = "f0443578"
	)
	agg := &londobell.TransferLargeAmountList{
		TotalCount: 245116,
		TransferLargeAmount: []*londobell.ActorMessages{
			laAggRow(6409152, cid1, "root-1", from, to, laThreshold, "Send", 1),
			// 同一行在多库里各出现一次（线上会把边界重复行算两次）⇒ 同键两行
			laAggRow(6409152, cid1, "root-1", from, to, laThreshold, "Send", 1),
		},
	}
	pg := []*bo.LargeTransferRow{
		laPgRow(6409152, cid1, "root-1", from, to, laThreshold, "Send", 1),
		laPgRow(6409152, cid1, "root-1", from, to, laThreshold, "Send", 1),
	}

	r := CompareLargeAmount(laParams, agg, pg, 245116, 10)
	if !r.PassDetailed(true, true, true) {
		t.Fatalf("应 PASS，实际: diff=%d aggOnly=%d pgOnly=%d order=%d sort=%d total=%v",
			r.DiffCount, r.AggOnlyCount(), r.PgOnlyCount(), r.OrderDiffCount, r.SortViolations, r.TotalDiff)
	}
	if r.AggTotal != 245116 || r.PgTotal != 245116 || r.TotalDiff {
		t.Errorf("TotalCount 应两侧相等: agg=%d pg=%d diff=%v", r.AggTotal, r.PgTotal, r.TotalDiff)
	}
}

// 金额不等（含丢精度场景）→ FAIL，且样例必须打印两侧原始文本；地址前缀形态差异只算「仅文本不同」。
func TestCompareLargeAmountValueMismatchAndAddressForm(t *testing.T) {
	agg := &londobell.TransferLargeAmountList{
		TotalCount: 1,
		TransferLargeAmount: []*londobell.ActorMessages{
			// 聚合器侧给的是不带前缀的 robust 地址形态
			laAggRow(100, "cid-a", "", "410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa", "0443578", laThreshold, "Send", 1),
		},
	}
	pg := []*bo.LargeTransferRow{
		// PG 侧带前缀；金额少了最后一位（真错）
		laPgRow(100, "cid-a", "", "f410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa", "f0443578", "1000000000000000000000", "Send", 1),
	}

	r := CompareLargeAmount(laParams, agg, pg, 1, 10)
	if r.PassDetailed(false, true, false) {
		t.Fatal("金额不等必须 FAIL")
	}
	if r.ValueDiffCount != 1 {
		t.Errorf("应有 1 条数值不等，得到 %d", r.ValueDiffCount)
	}
	// From 与 To 都只是缺前缀 ⇒ 两条 format-only，不算数值不等。
	if r.FormatOnlyDiff != 2 {
		t.Errorf("地址带不带前缀应记为 format-only（2 条：From/To），得到 %d（valueDiff=%d）", r.FormatOnlyDiff, r.ValueDiffCount)
	}
	var found bool
	for _, d := range r.Diffs {
		if d.Field == "Value" && !d.ValueEq && d.Agg == laThreshold && d.Pg == "1000000000000000000000" {
			found = true
		}
	}
	if !found {
		t.Errorf("差异样例必须打印两侧金额原文，实际: %+v", r.Diffs)
	}
}

// 只有地址原文形态不同（语义相同）⇒ 只记 format-only：-strict-format=false 时放行。
func TestCompareLargeAmountAddressFormOnly(t *testing.T) {
	agg := &londobell.TransferLargeAmountList{
		TotalCount:          1,
		TransferLargeAmount: []*londobell.ActorMessages{laAggRow(100, "cid-a", "", "410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa", "0443578", laThreshold, "Send", 1)},
	}
	pg := []*bo.LargeTransferRow{laPgRow(100, "cid-a", "", "f410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa", "f0443578", laThreshold, "Send", 1)}

	r := CompareLargeAmount(laParams, agg, pg, 1, 10)
	if r.ValueDiffCount != 0 {
		t.Errorf("语义相同不应算数值不等，得到 %d", r.ValueDiffCount)
	}
	if r.FormatOnlyDiff == 0 {
		t.Fatal("原文形态不同应记 format-only 差异")
	}
	if !r.PassDetailed(false, true, false) {
		t.Error("-strict-format=false 时应放行（形态差异不影响接口输出：Address() 会补前缀）")
	}
	if r.PassDetailed(true, true, false) {
		t.Error("-strict-format=true 时形态差异应判失败")
	}
}

// 行缺失（重数不同）→ aggOnly/pgOnly，逐行计数。
func TestCompareLargeAmountMissingRows(t *testing.T) {
	agg := &londobell.TransferLargeAmountList{
		TotalCount: 2,
		TransferLargeAmount: []*londobell.ActorMessages{
			laAggRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1),
			laAggRow(99, "cid-b", "", "from", "to", laThreshold, "Send", 1),
		},
	}
	// PG 侧少 cid-b、多 cid-c
	pg := []*bo.LargeTransferRow{
		laPgRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1),
		laPgRow(98, "cid-c", "", "from", "to", laThreshold, "Send", 1),
	}

	r := CompareLargeAmount(laParams, agg, pg, 2, 10)
	if r.AggOnlyCount() != 1 || r.PgOnlyCount() != 1 {
		t.Fatalf("应各 1 行缺失/多出，得到 aggOnly=%d pgOnly=%d", r.AggOnlyCount(), r.PgOnlyCount())
	}
	if !strings.Contains(r.AggOnly()[0], "cid-b") || !strings.Contains(r.PgOnly()[0], "cid-c") {
		t.Errorf("点位样例应点名 key：aggOnly=%v pgOnly=%v", r.AggOnly(), r.PgOnly())
	}
	if r.TotalDiff {
		t.Errorf("两侧 TotalCount 都给 2，行集合的差异不该被算成 total_diff（实际 agg=%d pg=%d）", r.AggTotal, r.PgTotal)
	}
	if r.OrderDiffCount != 0 {
		t.Errorf("行集合不一致时不该再比行序，得到 %d", r.OrderDiffCount)
	}
}

// 同高度内行序不同 → 只记 order_diff（已知差异）：默认放行，-strict-order 判失败。
func TestCompareLargeAmountIntraEpochOrderKnownDiff(t *testing.T) {
	agg := &londobell.TransferLargeAmountList{
		TotalCount: 2,
		TransferLargeAmount: []*londobell.ActorMessages{
			// 线上同高度内的顺序由各库返回顺序决定（未定义）：这里给 cid-b 在前
			laAggRow(100, "cid-b", "", "from", "to", laThreshold, "Send", 1),
			laAggRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1),
		},
	}
	// PG 侧按 (epoch desc, cid asc)：cid-a 在前
	pg := []*bo.LargeTransferRow{
		laPgRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1),
		laPgRow(100, "cid-b", "", "from", "to", laThreshold, "Send", 1),
	}

	r := CompareLargeAmount(laParams, agg, pg, 2, 10)
	if r.AggOnlyCount() != 0 || r.PgOnlyCount() != 0 || r.DiffCount != 0 {
		t.Fatalf("行集合一致，不应有缺失/字段差异: aggOnly=%d pgOnly=%d diff=%d", r.AggOnlyCount(), r.PgOnlyCount(), r.DiffCount)
	}
	if r.OrderDiffCount != 2 {
		t.Errorf("应记 2 处同高度内行序差异，得到 %d", r.OrderDiffCount)
	}
	if !r.PassDetailed(true, true, false) {
		t.Error("行序差异默认不应判失败（线上同高度内行序未定义）")
	}
	if r.PassDetailed(true, true, true) {
		t.Error("-strict-order 时行序差异应判失败")
	}
	if len(r.OrderDiffs) != 2 || !strings.Contains(r.OrderDiffs[0], "cid-a") {
		t.Errorf("行序差异样例应带位置与两侧 key，实际: %v", r.OrderDiffs)
	}
}

// TotalCount 不等：默认致命（口径判据），-lenient-total 降级为提示。
func TestCompareLargeAmountTotalDiff(t *testing.T) {
	agg := &londobell.TransferLargeAmountList{TotalCount: 7}
	r := CompareLargeAmount(laParams, agg, nil, 245116, 10)
	if !r.TotalDiff {
		t.Fatal("TotalCount 不等必须被记录")
	}
	if r.AggTotal != 7 || r.PgTotal != 245116 {
		t.Errorf("两侧 TotalCount 应各自记录，得到 agg=%d pg=%d", r.AggTotal, r.PgTotal)
	}
	if r.PassDetailed(true, true, false) {
		t.Error("默认（strictTotal=true）TotalCount 不等应判失败")
	}
	if !r.PassDetailed(true, false, false) {
		t.Error("-lenient-total 时 TotalCount 差额应只作提示")
	}
}

// -pg-only 自检：不调聚合器（agg=nil）、不判 total_diff、仍做 PG 侧定序检查。
func TestCompareLargeAmountPgOnly(t *testing.T) {
	pg := []*bo.LargeTransferRow{
		laPgRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1),
		laPgRow(99, "cid-b", "", "from", "to", laThreshold, "Send", 2),
	}
	p := laParams
	p.PgOnly = true

	r := CompareLargeAmount(p, nil, pg, 245116, 10)
	if !r.PgOnlyMode {
		t.Error("必须标记 PgOnlyMode（避免把「没比」看成「比过了」）")
	}
	if r.PgOnlyCount() != 0 || r.AggOnlyCount() != 0 {
		t.Errorf("pg-only 不应把聚合器侧的空结果算成缺口: aggOnly=%d pgOnly=%d", r.AggOnlyCount(), r.PgOnlyCount())
	}
	if r.TotalDiff {
		t.Error("pg-only 没有聚合器对照值，不应报 total_diff")
	}
	if r.PgRows != 2 || r.PgTotal != 245116 {
		t.Errorf("应输出 PG 侧行数与 TotalCount，得到 rows=%d total=%d", r.PgRows, r.PgTotal)
	}
	if !r.PassDetailed(true, true, true) {
		t.Error("pg-only 且 PG 侧自检通过时应 PASS")
	}
}

// PG 自己的定序（epoch desc）被破坏 ⇒ 永远致命（与「同高度内行序」是两回事，不能混）。
func TestCompareLargeAmountPgSortViolation(t *testing.T) {
	pg := []*bo.LargeTransferRow{
		laPgRow(99, "cid-a", "", "from", "to", laThreshold, "Send", 1),
		laPgRow(100, "cid-b", "", "from", "to", laThreshold, "Send", 1), // epoch 反弹
	}
	p := laParams
	p.PgOnly = true

	r := CompareLargeAmount(p, nil, pg, 2, 10)
	if r.SortViolations != 1 {
		t.Fatalf("应记 1 条定序违规，得到 %d", r.SortViolations)
	}
	if len(r.SortSamples) != 1 {
		t.Errorf("应打印定序违规样例，实际: %v", r.SortSamples)
	}
	if r.PassDetailed(true, true, true) {
		t.Error("PG 定序违规必须判失败（连 pg-only 也一样）")
	}
}

// 管线的 $project 里没有 SignedCid/ExitCode ⇒ PG 侧填了就会被抓出来。
func TestCompareLargeAmountZeroValueFields(t *testing.T) {
	agg := &londobell.TransferLargeAmountList{
		TotalCount:          1,
		TransferLargeAmount: []*londobell.ActorMessages{laAggRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1)},
	}
	pg := []*bo.LargeTransferRow{laPgRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1)}

	r := CompareLargeAmount(laParams, agg, pg, 1, 10)
	if r.DiffCount != 0 {
		t.Errorf("两侧都为零值时应无差异，实际: %+v", r.Diffs)
	}

	// 反向 A：聚合器侧带了 SignedCid（说明线上投影变了）⇒ 键相同、字段级差异必须报出来。
	agg.TransferLargeAmount[0].SignedCid = "bafy2bzaced-signed"
	r = CompareLargeAmount(laParams, agg, pg, 1, 10)
	if r.ValueDiffCount == 0 {
		t.Errorf("SignedCid 不一致必须报字段差异，实际: %+v", r.Diffs)
	}
	var signedFound bool
	for _, d := range r.Diffs {
		if d.Field == "SignedCid" {
			signedFound = true
		}
	}
	if !signedFound {
		t.Errorf("差异样例必须点名 SignedCid，实际: %+v", r.Diffs)
	}

	// 反向 B：Cid 本身不一致 ⇒ 行身份键不同，落成「只在一边存在」而不是字段差异（样例里带 key 可定位）。
	agg.TransferLargeAmount[0].SignedCid = ""
	agg.TransferLargeAmount[0].Cid = "bafy2bzaced-agg"
	pg[0].Cid = "bafy2bzaced-pg"
	r = CompareLargeAmount(laParams, agg, pg, 1, 10)
	if r.AggOnlyCount() == 0 || r.PgOnlyCount() == 0 {
		t.Errorf("Cid 不一致应落成行缺失（键不同），实际: aggOnly=%d pgOnly=%d", r.AggOnlyCount(), r.PgOnlyCount())
	}
}

// 空页（聚合器 data:null ⇒ agg=nil）而 PG 有行 ⇒ 真差异（PG 覆盖面更大），不能静默放过。
func TestCompareLargeAmountAggNilButPgHasRows(t *testing.T) {
	pg := []*bo.LargeTransferRow{laPgRow(100, "cid-a", "", "from", "to", laThreshold, "Send", 1)}
	r := CompareLargeAmount(laParams, nil, pg, 245116, 10)
	if r.PgOnlyCount() != 1 {
		t.Fatalf("应记 1 行只在 PG 侧，得到 %d", r.PgOnlyCount())
	}
	if r.PassDetailed(true, true, false) {
		t.Error("聚合器空页而 PG 有行必须判失败")
	}
}

// 两侧都空（空表 + 聚合器空页）⇒ PASS：空不是错误，形态一致即可。
func TestCompareLargeAmountBothEmpty(t *testing.T) {
	r := CompareLargeAmount(laParams, nil, nil, 0, 10)
	if !r.PassDetailed(true, true, true) {
		t.Fatalf("两侧都空应 PASS，实际: %+v", r)
	}
	if r.PgRows != 0 || r.PgTotal != 0 || r.TotalDiff {
		t.Errorf("两侧都空时各项应为 0: %+v", r)
	}
}

// 端点的两个能力位：大额转账不需要高度区间、且必须出现在可选端点清单里。
func TestLargeAmountEndpointRegistry(t *testing.T) {
	if NeedsEpochRange(EndpointLargeAmount) {
		t.Error("大额转账端点只吃 index/limit，不该要求 -start/-end")
	}
	if !NeedsEpochRange(EndpointMinerBlockReward) {
		t.Error("三个统计端点仍应按高度区间取数")
	}
	var found bool
	for _, e := range ValidEndpoints() {
		if e == EndpointLargeAmount {
			found = true
		}
	}
	if !found {
		t.Errorf("ValidEndpoints 必须包含 %s，实际: %v", EndpointLargeAmount, ValidEndpoints())
	}
	for _, e := range AllEndpoints() {
		if e == EndpointLargeAmount {
			t.Error("默认端点集合不应包含大额转账（线上极慢，且不按区间取数）")
		}
	}
}
