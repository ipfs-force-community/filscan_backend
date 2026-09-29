package dal

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ----- 一个「会说 postgres 方言、会真的按 where 过滤」的桩驱动 -----
//
// 目的：让 DAL 在**不连任何数据库**的前提下，既暴露真正下发出去的 SQL/参数，
// 又按 SQL 里写的 where 条件给出结果集 ⇒ 区间语义可以被真正验证（而不是只比字符串）。

// epochCmp 匹配 `epoch >= $1` / `epoch < $2` 这类比较，捕获 (运算符, 参数序号)
var epochCmp = regexp.MustCompile(`epoch\s*(>=|<=|>|<)\s*\$(\d+)`)

// stubRecorder：桩连接的记录器。table 是「假 postgres 里的 chain.miner_rewards」，
// 键为高度、值为该高度爆块的矿工。distinct 表示 select 语句是否去重。
type stubRecorder struct {
	table   map[int64][]string
	queries []string
	args    [][]driver.Value
}

func (r *stubRecorder) lastQuery() (string, []driver.Value) {
	if len(r.queries) == 0 {
		return "", nil
	}
	return r.queries[len(r.queries)-1], r.args[len(r.args)-1]
}

// evaluate 按 SQL 里写出的 where 条件过滤假表 —— 「区间是闭还是开」由 SQL 决定
func (r *stubRecorder) evaluate(query string, args []driver.Value) []driver.Value {
	bounds := map[int]driver.Value{} // $n ⇒ 值（postgres 的 $n 是 1-based）
	for i, v := range args {
		bounds[i+1] = v
	}

	var epochs []int64
	for e := range r.table {
		epochs = append(epochs, e)
	}
	sort.Slice(epochs, func(i, j int) bool { return epochs[i] < epochs[j] })

	match := epochCmp.FindAllStringSubmatch(query, -1)
	var out []driver.Value
	seen := map[string]bool{}
	for _, e := range epochs {
		for _, miner := range r.table[e] {
			ok := true
			for _, m := range match {
				n, err := strconv.Atoi(m[2])
				if err != nil {
					ok = false
					break
				}
				bound, isInt := bounds[n].(int64)
				if !isInt {
					ok = false
					break
				}
				switch m[1] {
				case ">=":
					ok = e >= bound
				case ">":
					ok = e > bound
				case "<=":
					ok = e <= bound
				case "<":
					ok = e < bound
				}
				if !ok {
					break
				}
			}
			if ok && !seen[miner] {
				seen[miner] = true
				out = append(out, miner)
			}
		}
	}
	return out
}

type stubDriver struct{ rec *stubRecorder }

func (d *stubDriver) Open(string) (driver.Conn, error) { return &stubConn{rec: d.rec}, nil }

type stubConn struct{ rec *stubRecorder }

func (c *stubConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("stubConn: Prepare 未实现")
}
func (c *stubConn) Close() error              { return nil }
func (c *stubConn) Begin() (driver.Tx, error) { return nil, errors.New("stubConn: Begin 未实现") }

func (c *stubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := make([]driver.Value, 0, len(args))
	for _, a := range args {
		values = append(values, a.Value)
	}
	c.rec.queries = append(c.rec.queries, query)
	c.rec.args = append(c.rec.args, values)
	return &stubRows{rows: c.rec.evaluate(query, values)}, nil
}

type stubRows struct {
	rows []driver.Value
	i    int
}

func (r *stubRows) Columns() []string { return []string{"miner"} }
func (r *stubRows) Close() error      { return nil }
func (r *stubRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	dest[0] = r.rows[r.i]
	r.i++
	return nil
}

var stubSeq atomic.Int64

// openStubDB 打开一个走桩驱动的 gorm.DB（不连库、不下发任何真实 SQL）
func openStubDB(t *testing.T, rec *stubRecorder) *gorm.DB {
	t.Helper()
	name := fmt.Sprintf("filscan_reward_stub_%d", stubSeq.Add(1))
	sql.Register(name, &stubDriver{rec: rec})
	db, err := gorm.Open(
		postgres.New(postgres.Config{DriverName: name, DSN: "stub"}),
		&gorm.Config{Logger: gormlogger.Discard},
	)
	require.NoError(t, err)
	return db
}

// 回归测试：GetRewardMiners 的入参是 LCRCRange（**左闭右闭**，见 pkg/chain/epoch.go:143 与
// syncer/context.go:37「批量同步区间，左闭右闭」），调用方 calc-miner-agg-reward 传的是
// ctx.Epochs()，即本批要覆盖的高度。旧 SQL 右端写成 `epoch < ?` ⇒ 每批**最后一个高度**
// 爆块的矿工永远查不到，这些矿工的累计统计（miner_agg_rewards）就漏更新一次。
//
// 本测试用一个「会真按 where 过滤」的桩驱动跑真实 DAL 代码：旧 SQL 下 102 高度的矿工
// 不会出现在结果里，必然失败（红）；改成 <= 后与「左闭右闭」的预期一致。
func TestGetRewardMinersUsesClosedEpochRange(t *testing.T) {
	rec := &stubRecorder{table: map[int64][]string{
		99:  {"f0999"}, // 区间之前，不该被选中
		100: {"f0100"}, // 区间左端（含）
		101: {"f0101"},
		102: {"f0102", "f0333"}, // 区间右端：旧实现会漏掉这两个矿工
		103: {"f0998"},          // 区间之后，不该被选中
	}}
	db := openStubDB(t, rec)

	miners, err := NewRewardTaskDal(db).GetRewardMiners(context.Background(), chain.NewLCRCRange(100, 102))
	require.NoError(t, err)
	require.Equal(t, []string{"f0100", "f0101", "f0102", "f0333"}, miners,
		"左闭右闭区间 [100,102] 的矿工必须全部返回（含右端 102 高度的矿工）")

	query, args := rec.lastQuery()
	require.Contains(t, query, "epoch >= $1")
	require.Contains(t, query, "epoch <= $2", "右端必须是闭区间")
	require.NotContains(t, query, "epoch < $2", "右端写成开区间会漏掉每批最后一个高度")
	require.Equal(t, []driver.Value{int64(100), int64(102)}, args, "参数就是 LCRCRange 的两端")
}

// 单个高度（GteBegin == LteEnd，LCRCRange 退化成「一个高度」）时也必须查到该高度的矿工
func TestGetRewardMinersSingleEpochRangeIsInclusive(t *testing.T) {
	rec := &stubRecorder{table: map[int64][]string{
		100: {"f0100"},
		101: {"f0101"},
	}}
	db := openStubDB(t, rec)

	miners, err := NewRewardTaskDal(db).GetRewardMiners(context.Background(), chain.NewLCRCRange(100, 100))
	require.NoError(t, err)
	require.Equal(t, []string{"f0100"}, miners, "单高度区间不能返回空集")
}

// 区间内没有任何矿工爆块（空洞高度）时返回空集且不报错
func TestGetRewardMinersEmptyRange(t *testing.T) {
	rec := &stubRecorder{table: map[int64][]string{
		100: {"f0100"},
	}}
	db := openStubDB(t, rec)

	miners, err := NewRewardTaskDal(db).GetRewardMiners(context.Background(), chain.NewLCRCRange(200, 300))
	require.NoError(t, err)
	require.Empty(t, miners)
}
