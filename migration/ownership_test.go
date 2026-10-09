package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// ownershipConventionFrom 是「建表迁移必须自带属主与授权修正」这条约定的生效起点（migration 序号）。
//
// 为什么不追溯历史：37 之前的建表迁移是历史文件，它们建的表在库里本来就归应用账号（实测零例外），
// 补写属主段只是空转；而 37/38 这两张表恰恰因为「用超级用户跑」而落到了 postgres 名下。
// 自 37 起按棘轮式判据强制：新建表必须自带 do 块（可从同 schema 既有表推导应用账号），否则单测红。
const ownershipConventionFrom = 37

var migrationFileRe = regexp.MustCompile(`^(\d+)\..*\.sql$`)

// TestMigrationsFixOwnership 机检：自 ownershipConventionFrom 起，任何含 `create table` 的迁移
// 都必须同时带属主修正语句。
//
// 为什么需要（2026-10-09 主网事故）：迁移若以超级用户执行（PG 主机上 `su postgres -c "psql -f …"`），
// 新表会归 postgres 且不授任何权限；而同步器用的是应用账号，于是计算器的写入与 RollBack 在新表上
// permission denied —— 一次链分叉触发的回滚永远完不成，chain 基础管线原地重试 1.7 小时，连带
// 8 条同步器与 7 张逐高度表停摆；进程既不崩也不退出，只刷 ERROR，所以没人及时发现。
// 这条约定不能靠人记得：建表迁移必须自足（无论以哪个账号执行，新建对象都要归应用账号并授只读权限），
// 因此在此拦住。写法见 migration/37 与 38 末尾的 do 块。
func TestMigrationsFixOwnership(t *testing.T) {
	files, err := filepath.Glob("*.sql")
	if err != nil {
		t.Fatalf("glob migration/*.sql: %v", err)
	}
	if len(files) == 0 {
		t.Fatal("migration/*.sql 一个都没找到 ⇒ 机检空转（工作目录应为 migration/），判据失效")
	}

	checked, legacy := 0, 0
	for _, f := range files {
		m := migrationFileRe.FindStringSubmatch(f)
		if m == nil {
			continue // 不符合 <序号>.<名>.sql 命名的文件不是迁移，忽略
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("%s: 序号无法解析: %v", f, err)
		}

		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		sql := strings.ToLower(string(b))
		if !strings.Contains(sql, "create table") {
			continue
		}
		if n < ownershipConventionFrom {
			legacy++
			continue
		}

		checked++
		if !strings.Contains(sql, "owner to") {
			t.Errorf("%s 含 create table 但没有属主修正语句（owner to）。\n"+
				"迁移必须自足：新建的表/索引要归应用账号（可用 do 块从同 schema 既有表推导），"+
				"否则应用账号在写入或 RollBack 时会 permission denied 并把基础管线卡死。\n"+
				"参照 migration/37.reward_stream_recipient_epoch.sql 末尾的写法。", f)
		}
	}

	if checked == 0 {
		t.Fatalf("没有任何 migration >= %d 的建表迁移被检查到 ⇒ 机检空转，判据失效", ownershipConventionFrom)
	}
	t.Logf("已检查 %d 个建表迁移（序号 >= %d），跳过 %d 个历史文件", checked, ownershipConventionFrom, legacy)
}
