package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/go-resty/resty/v2"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	londobellimpl "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/impl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 端到端集成测试（默认跳过）：把「真 PG 表 chain.large_transfers」与「真聚合器客户端（打到本地 stub）」
// 两条路径各跑一次，逐字段断言相等。这是把大额转账 PG 读路径从单测的假数据升级到真解析链的验证 ——
// 覆盖 decimal 的 numeric(38,0) → decimal.Decimal 扫描（**大额 attoFIL 不许丢精度**）、
// JSON 解包（管线的 lowerCamelCase 键 + 带引号的金额字符串）、coalesce(NULL→零值) 与 index/limit 折算。
//
// stub 返回的是**聚合器管线的真实形态**（transfer_message_for_large_amount.js 的 $project）：
// 只有 Cid/RootCid/Epoch/From/To/Value/Method/Depth 八个键（没有 SignedCid / ExitCode / IsBlock）。
//
// 跑法（一次性库，库名必须含 test/probe，防止误指线上）：
//
//	FILSCAN_PG_PARITY_DSN='host=/tmp user=postgres dbname=filscan_probe_large_amount sslmode=disable' \
//	  go test ./modules/filscan/biz/browser/ -run TestPgLargeAmountAggMatchesAggregator -v
func TestPgLargeAmountAggMatchesAggregator(t *testing.T) {
	dsn := os.Getenv("FILSCAN_PG_PARITY_DSN")
	if dsn == "" {
		t.Skip("未设置 FILSCAN_PG_PARITY_DSN，跳过（集成测试需要一次性 PG）")
	}
	if !strings.Contains(dsn, "test") && !strings.Contains(dsn, "probe") {
		t.Skipf("DSN 必须指向一次性库（库名含 test/probe），当前: %s", dsn)
	}

	db, err := openProbeDB(dsn)
	if err != nil {
		t.Fatalf("连接 PG 失败: %s", err)
	}
	if err := prepareProbeLargeTransferTable(db); err != nil {
		t.Fatalf("准备探针表失败: %s", err)
	}

	// 聚合器 stub：按线上管线的 (skip=index*limit, limit) 语义分页，返回管线形态的 JSON。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/aggregators/transfer_message_for_largeAmount") {
			http.NotFound(w, r)
			return
		}
		var req struct {
			Index int64 `json:"index"`
			Limit int64 `json:"limit"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		// 与线上 multi-query/query.go:296-300 完全一致：index 是页码；两个都为 0 时不设分页（取全量）。
		skip, limit := int64(0), int64(math.MaxInt64)
		if req.Index > 0 || req.Limit > 0 {
			skip = req.Index * req.Limit
			limit = req.Limit
		}

		rows := make([]map[string]interface{}, 0, len(probeLargeTransfers))
		for i, row := range probeLargeTransfers {
			if int64(i) < skip || int64(len(rows)) >= limit {
				continue
			}
			item := map[string]interface{}{
				"Cid":     row.cid,
				"Epoch":   row.epoch,
				"From":    row.from,
				"To":      row.to,
				"Value":   row.value,
				"Method":  row.method,
				"RootCid": row.rootCid,
			}
			if row.depth != nil {
				item["Depth"] = *row.depth
			}
			rows = append(rows, item)
		}
		w.Header().Set("Content-Type", "application/json")
		// 线上控制器在「一条都没取到」时**提前返回**，data 是 null（不是空数组）——
		// filscan 侧 bindResult 把 null 解成 nil，于是 acl/biz 走「无数据」分支。这里照抄该形态。
		if len(rows) == 0 {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": "0", "msg": "", "data": nil})
			return
		}
		// 聚合器真实的键名形态：totalCount / transferMessagesForLargeAmount（lowerCamelCase）
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"code": "0", "msg": "",
			"data": map[string]interface{}{
				"totalCount":                     len(probeLargeTransfers),
				"transferMessagesForLargeAmount": rows,
			},
		})
	}))
	defer srv.Close()

	agg := londobellimpl.NewLondobellAggImpl(srv.URL, resty.New())
	conf := &config.Config{Feature: &config.Feature{LargeAmountReadFromPg: boolPtrLocal(true)}}
	wrapped := NewPgLargeAmountAgg(agg, db, conf)
	if _, ok := wrapped.(*pgLargeAmountAgg); !ok {
		t.Fatalf("开关已开但 agg 未被装饰，得到 %T", wrapped)
	}

	ctx := context.Background()
	// 探针数据按 epoch 倒序插入 PG 的顺序；stub 也按同序返回，故这里比的是「同一页」。
	// （同高度内行序差异由 rewardparity 的单测覆盖，这里不引入不确定性。）
	for _, page := range []struct{ index, limit int64 }{{0, 10}, {0, 2}, {1, 2}, {100, 2}, {0, 0}} {
		filters := types.Filters{Index: page.index, Limit: page.limit}
		aggList, err := agg.TransferLargeAmount(ctx, filters)
		if err != nil {
			t.Fatalf("聚合器侧取数失败 (index=%d limit=%d): %s", page.index, page.limit, err)
		}
		pgList, err := wrapped.TransferLargeAmount(ctx, filters)
		if err != nil {
			t.Fatalf("PG 侧取数失败 (index=%d limit=%d): %s", page.index, page.limit, err)
		}
		if aggList == nil || pgList == nil {
			if aggList != nil || pgList != nil {
				t.Fatalf("(index=%d limit=%d) 两侧空页形态不一致: 聚合器 %v vs PG %v", page.index, page.limit, aggList, pgList)
			}
			continue
		}
		if aggList.TotalCount != pgList.TotalCount {
			t.Errorf("(index=%d limit=%d) TotalCount 不一致: 聚合器 %d vs PG %d", page.index, page.limit, aggList.TotalCount, pgList.TotalCount)
		}
		if len(aggList.TransferLargeAmount) != len(pgList.TransferLargeAmount) {
			t.Fatalf("(index=%d limit=%d) 行数不一致: 聚合器 %d vs PG %d", page.index, page.limit,
				len(aggList.TransferLargeAmount), len(pgList.TransferLargeAmount))
		}
		for i := range aggList.TransferLargeAmount {
			a, p := aggList.TransferLargeAmount[i], pgList.TransferLargeAmount[i]
			if a.Cid != p.Cid || a.RootCid != p.RootCid || a.Epoch != p.Epoch ||
				a.From != p.From || a.To != p.To || a.Value.String() != p.Value.String() ||
				a.Method != p.Method || a.Depth != p.Depth {
				t.Errorf("(index=%d limit=%d) 第 %d 行不一致:\n  聚合器 %+v (Value=%s)\n  PG     %+v (Value=%s)",
					page.index, page.limit, i, a, a.Value.String(), p, p.Value.String())
			}
			// 管线的 $project 里没有这两个键 ⇒ 两侧都必须为零值（SignedCid 被并进 Cid、ExitCode 由 $match=0 限定）。
			if p.SignedCid != "" || p.ExitCode != 0 {
				t.Errorf("(index=%d limit=%d) 第 %d 行 PG 侧 SignedCid/ExitCode 必须是零值: %+v", page.index, page.limit, i, p)
			}
		}
	}
}

// probeLargeTransfer 探针行（与 prepareProbeLargeTransferTable 的插入、stub 的返回一一对应）。
// depth 用指针是因为库里有 NULL（== 聚合器侧 null → Go 零值 0）。
type probeLargeTransfer struct {
	epoch   int64
	cid     string
	rootCid string
	from    string
	to      string
	value   string
	method  string
	depth   *int64
}

func int64PtrLocal(v int64) *int64 { return &v }

// 探针数据刻意覆盖：
//
//  1. 大额 attoFIL：门槛值 1e22（23 位）与 38 位极值 —— 过一次 float64 就会失真；
//  2. 同 (epoch, cid) 重复行（跨库边界重复 + 竞争区块）—— 重数是口径的一部分，**不许被去重**；
//  3. root_cid / method / depth 为 NULL —— coalesce 后应与聚合器的空值形态一致；
//  4. 同一个高度多行 —— 定序 (epoch desc, cid asc) 要在 stub 与 PG 两侧表现一致。
var probeLargeTransfers = []probeLargeTransfer{
	{6409152, "bafy2bzaced-la-1", "bafy2bzaced-root-1", "f410fc6jo2qwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa", "f0443578", "99999999999999999999999999999999999999", "Send", int64PtrLocal(1)},
	{6409150, "bafy2bzaced-la-2", "", "f1jqwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa7xgyi", "f2abc", "10000000000000000000000", "", nil},
	{6409150, "bafy2bzaced-la-2", "", "f1jqwfposuoq2zkb6fjjrfu6uw7i6x3x7pxa7xgyi", "f2abc", "10000000000000000000000", "", nil}, // 边界重复行
	{6409149, "bafy2bzaced-la-3", "bafy2bzaced-root-3", "f3aaaa", "f3bbbb", "123456789012345678901234567890", "InvokeContract", int64PtrLocal(4)},
}

// prepareProbeLargeTransferTable 复刻 migration/35.large_transfers.sql（含「故意不建主键/唯一索引」）。
func prepareProbeLargeTransferTable(db *gorm.DB) error {
	statements := []string{
		`create schema if not exists chain`,
		`drop table if exists chain.large_transfers cascade`,
		`create table chain.large_transfers (
			epoch     bigint       not null,
			cid       text         not null,
			root_cid  text,
			from_addr text,
			to_addr   text,
			value     numeric(38, 0),
			method    text,
			depth     integer
		)`,
		`create index large_transfers_epoch_index on chain.large_transfers (epoch)`,
	}
	for _, row := range probeLargeTransfers {
		depth := "NULL"
		if row.depth != nil {
			depth = fmt.Sprintf("%d", *row.depth)
		}
		statements = append(statements, fmt.Sprintf(
			`insert into chain.large_transfers (epoch, cid, root_cid, from_addr, to_addr, value, method, depth)
			 values (%d, '%s', nullif('%s',''), '%s', '%s', %s, nullif('%s',''), %s)`,
			row.epoch, row.cid, row.rootCid, row.from, row.to, row.value, row.method, depth))
	}
	for _, s := range statements {
		if err := db.Exec(s).Error; err != nil {
			return fmt.Errorf("探针表语句失败: %w", err)
		}
	}
	return nil
}

// openProbeDB 打开一次性探针库（只在集成测试里用；DSN 由 FILSCAN_PG_PARITY_DSN 给出）。
func openProbeDB(dsn string) (*gorm.DB, error) {
	return gorm.Open(postgres.Open(dsn), &gorm.Config{})
}
