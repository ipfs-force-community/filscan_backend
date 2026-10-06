package dal

import (
	"strings"
	"testing"
)

// 本文件是 ORDER BY 注入（本次生产事故）的回归测试。
//
// 每处注入点的 DAL 方法都只通过 order_whitelist.go 里的 resolveOrder / buildOrderClause
// 把「请求里的 field/sort」变成 SQL 片段；基础 SQL 是编译期常量、不含用户输入。
// 因此只要证明这两个构造函数在收到恶意 field/sort 时「要么报错、要么只回退到白名单里的常量」，
// 就等价于证明最终拼出的 SQL 不含任何载荷。

// injectionPayloads 覆盖本次事故用到的报错型注入以及典型变体。
var injectionPayloads = []string{
	"created_at DESC, (select pg_read_file('/etc/passwd'))::int",
	"quality_adj_power desc, cast(('x'||(select pg_read_file('/etc/passwd'))) as int) desc",
	"1=1",
	"id;drop table x",
	"field) union select pg_sleep(10)--",
	"transfer_count, (select pg_sleep(5))",
}

// forbiddenFragments 任一出现在构造结果里即视为注入未被拦住。
var forbiddenFragments = []string{
	"pg_read_file", "pg_sleep", "cast(", "1=1", ";", "(", ")",
	"union", "drop table", "--", "select",
}

func assertNoInjection(t *testing.T, out string) {
	t.Helper()
	for _, bad := range forbiddenFragments {
		if strings.Contains(strings.ToLower(out), bad) {
			t.Fatalf("SQL 片段里出现注入载荷特征 %q: %q", bad, out)
		}
	}
}

// orderSite 描述一处 ORDER BY 注入点：它使用的白名单、默认列/方向，以及一个合法字段样例。
type orderSite struct {
	name          string
	whitelist     map[string]string
	defaultColumn string
	defaultSort   string
	legalField    string
	legalColumn   string
}

// orderSites 覆盖全部 7 处注入点（3 处 fmt.Sprintf + 4 处模板 {{ }}）。
func orderSites() []orderSite {
	return []orderSite{
		// dal_evm_transfer.go:171 GetEvmTransferStatsList
		{"dal_evm_transfer.GetEvmTransferStatsList", evmTransferStatsOrderColumns, "acc_transfer_count", "desc", "transfer_count", "acc_transfer_count"},
		// dal_evm_transfer.go:255 GetEvmTransferList
		{"dal_evm_transfer.GetEvmTransferList", evmTransferListOrderColumns, "transfer_count", "desc", "user_count", "user_count"},
		// dal_evm_transaction.go:70 GetEvmTransactionStatsList
		{"dal_evm_transaction.GetEvmTransactionStatsList", evmTransactionStatsOrderColumns, "acc_transaction_count", "desc", "gas_cost", "acc_gas_cost"},
		// dal_biz_miner_rank.go:76 GetMinerRanks
		{"dal_biz_miner_rank.GetMinerRanks", minerRankOrderColumns, "quality_adj_power", "desc", "power_increase_24h", "b.quality_adj_power_change"},
		// dal_biz_miner_rank.go:149 GetMinerPowerRanks
		{"dal_biz_miner_rank.GetMinerPowerRanks", minerPowerRankOrderColumns, "quality_adj_power_change", "desc", "raw_power", "raw_byte_power"},
		// dal_biz_miner_rank.go:254 GetMinerRewardRanks
		{"dal_biz_miner_rank.GetMinerRewardRanks", minerRewardRankOrderColumns, "acc_reward", "desc", "winning_rate", "a.wining_rate"},
		// dal_biz_owner_rank.go:72 GetOwnerRanks
		{"dal_biz_owner_rank.GetOwnerRanks", ownerRankOrderColumns, "quality_adj_power", "desc", "rewards_ratio_24h", "b.reward_power_ratio"},
	}
}

// TestResolveOrderRejectsInjectionField 断言：field 传恶意载荷时，解析函数返回错误，
// 且不返回任何列名/方向（因此载荷不可能进入 SQL）。
// 这直接覆盖 4 处模板注入点（它们调用 resolveOrder）。
func TestResolveOrderRejectsInjectionField(t *testing.T) {
	for _, site := range orderSites() {
		site := site
		t.Run(site.name, func(t *testing.T) {
			for _, payload := range injectionPayloads {
				column, direction, err := resolveOrder(payload, "desc", site.whitelist, site.defaultColumn, site.defaultSort)
				if err == nil {
					t.Fatalf("field=%q 不在白名单，必须返回错误（fail-closed）", payload)
				}
				if column != "" || direction != "" {
					t.Fatalf("field=%q 被拒后不得返回列/方向: column=%q direction=%q", payload, column, direction)
				}
				if strings.Contains(column, payload) {
					t.Fatalf("载荷进入了列名: %q", column)
				}
			}
		})
	}
}

// TestBuildOrderClauseRejectsInjectionField 断言：经 buildOrderClause 构造出的 ORDER BY
// 片段，在恶意 field 下返回错误且为空串，绝不含 pg_read_file / 分号 / 括号表达式等。
// 这直接覆盖 3 处 fmt.Sprintf 注入点（它们调用 buildOrderClause）。
func TestBuildOrderClauseRejectsInjectionField(t *testing.T) {
	for _, site := range orderSites() {
		site := site
		t.Run(site.name, func(t *testing.T) {
			for _, payload := range injectionPayloads {
				clause, err := buildOrderClause(payload, "desc", site.whitelist, site.defaultColumn, site.defaultSort)
				if err == nil {
					t.Fatalf("field=%q 不在白名单，必须返回错误", payload)
				}
				if clause != "" {
					t.Fatalf("field=%q 被拒后不得返回 SQL 片段，实际: %q", payload, clause)
				}
				assertNoInjection(t, clause)
			}
		})
	}
}

// TestBuildOrderClauseInjectionInSortFallsBack 断言：sort 里的恶意载荷被丢弃，
// 结果永远是白名单列 + 默认方向（大小写不敏感，非法值回退）。
func TestBuildOrderClauseInjectionInSortFallsBack(t *testing.T) {
	for _, site := range orderSites() {
		site := site
		t.Run(site.name, func(t *testing.T) {
			for _, payload := range injectionPayloads {
				clause, err := buildOrderClause(site.legalField, payload, site.whitelist, site.defaultColumn, site.defaultSort)
				if err != nil {
					t.Fatalf("合法 field + 恶意 sort 不应报错: %v", err)
				}
				want := "ORDER BY " + site.legalColumn + " DESC\n"
				if clause != want {
					t.Fatalf("sort=%q 应回退到默认方向: got %q want %q", payload, clause, want)
				}
				assertNoInjection(t, clause)
			}
		})
	}
}

// TestBuildOrderClauseLegalFields 断言：白名单里的合法字段仍能正确拼成 ORDER BY。
func TestBuildOrderClauseLegalFields(t *testing.T) {
	for _, site := range orderSites() {
		site := site
		t.Run(site.name, func(t *testing.T) {
			for field, column := range site.whitelist {
				clause, err := buildOrderClause(field, "asc", site.whitelist, site.defaultColumn, site.defaultSort)
				if err != nil {
					t.Fatalf("合法 field=%q 不应报错: %v", field, err)
				}
				want := "ORDER BY " + column + " ASC\n"
				if clause != want {
					t.Fatalf("field=%q: got %q want %q", field, clause, want)
				}
				assertNoInjection(t, clause)
			}
			// 合法字段的大小写/空方向归一
			clause, err := buildOrderClause(site.legalField, "DESC", site.whitelist, site.defaultColumn, site.defaultSort)
			if err != nil || clause != "ORDER BY "+site.legalColumn+" DESC\n" {
				t.Fatalf("大写 DESC 应被接受: clause=%q err=%v", clause, err)
			}
		})
	}
}

// TestBuildOrderClauseDefaults 断言：未传 field 时使用默认列；未传/非法 sort 回退默认方向。
func TestBuildOrderClauseDefaults(t *testing.T) {
	for _, site := range orderSites() {
		site := site
		t.Run(site.name, func(t *testing.T) {
			clause, err := buildOrderClause("", "", site.whitelist, site.defaultColumn, site.defaultSort)
			if err != nil {
				t.Fatalf("空 field 不应报错: %v", err)
			}
			if want := "ORDER BY " + site.defaultColumn + " DESC\n"; clause != want {
				t.Fatalf("默认排序: got %q want %q", clause, want)
			}

			clause, err = buildOrderClause("", "asc", site.whitelist, site.defaultColumn, site.defaultSort)
			if err != nil {
				t.Fatalf("空 field 不应报错: %v", err)
			}
			if want := "ORDER BY " + site.defaultColumn + " ASC\n"; clause != want {
				t.Fatalf("默认列 + asc: got %q want %q", clause, want)
			}
		})
	}
}

// TestResolveSort 覆盖排序方向归一化与非法回退。
func TestResolveSort(t *testing.T) {
	cases := []struct {
		in, def, want string
	}{
		{"asc", "desc", "ASC"},
		{"ASC", "desc", "ASC"},
		{" Asc ", "desc", "ASC"},
		{"desc", "asc", "DESC"},
		{"DESC", "asc", "DESC"},
		{"", "asc", "ASC"},
		{"", "desc", "DESC"},
		{"desc; drop table x", "desc", "DESC"},
		{"asc, (select pg_read_file('/etc/passwd'))::int", "desc", "DESC"},
	}
	for _, c := range cases {
		if got := resolveSort(c.in, c.def); got != c.want {
			t.Fatalf("resolveSort(%q, %q)=%q want %q", c.in, c.def, got, c.want)
		}
	}
}
