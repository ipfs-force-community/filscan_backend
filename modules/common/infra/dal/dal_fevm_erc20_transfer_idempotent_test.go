package dal

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
)

// 这些用例只验「真正会发给 PG 的 SQL」，全程不连库、不依赖 PG 实例：
//   - 纯函数用例直接造数据；
//   - SQL 用例用 gorm DryRun + 捕获 logger.Trace；
//   - 守卫接线用例用一个最简假驱动回答守卫查询（返回「库里已有行」），证明已有的三元组真的被跳过。
//
// 为什么 erc20 写入必须走应用层「先查后跳」、且绝不能出现 ON CONFLICT：
// fevm.erc_20_transfers 上有 INSERT 规则（range_insert_action_rule ⇒
// fevm.action_erc_20_transfers_range_insert），PG 对带 INSERT/UPDATE 规则的表直接禁止
// ON CONFLICT（SQLSTATE 0A000: INSERT with ON CONFLICT clause cannot be used with table
// that has INSERT or UPDATE rules）——实测重跑已写过的 100 个高度全红、框架无限重试。
// 唯一键是 UNIQUE (epoch, cid, "index")（必须含分区键 epoch），所以认行的键是三元组。
// TestCreateERC20TransferBatchSQLHasNoOnConflict / TestCreateERC20SwapInfoBatchSQLHasNoOnConflict
// 就是防后来人再把 ON CONFLICT 加回来的回归护栏。

// sqlCapture 实现 gorm.io/gorm/logger.Interface，只把生成的 SQL 记下来。
type sqlCapture struct {
	mu   sync.Mutex
	sqls []string
}

func (c *sqlCapture) record(sql string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = append(c.sqls, sql)
}

func (c *sqlCapture) All() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sqls))
	copy(out, c.sqls)
	return out
}

func (c *sqlCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *sqlCapture) Info(context.Context, string, ...interface{})     {}
func (c *sqlCapture) Warn(context.Context, string, ...interface{})     {}
func (c *sqlCapture) Error(context.Context, string, ...interface{})    {}

func (c *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.record(sql)
}

// noopDriver/noopConn 是一个不接任何后端的空转驱动：DryRun 下 gorm 不会真正执行语句，
// 而 sql.Open 本身是惰性的，所以这个驱动只用于「开启事务」这类外壳调用；
// 一旦真的有语句被下发（Prepare），就直接报错 —— 用来证明用例确实没有碰库。
type noopDriver struct{}

func (noopDriver) Open(string) (driver.Conn, error) { return noopConn{}, nil }

type noopConn struct{}

var errNoopTouched = errors.New("noop driver: DryRun 用例不应真正执行任何语句")

func (noopConn) Prepare(string) (driver.Stmt, error) { return nil, errNoopTouched }
func (noopConn) Close() error                        { return nil }

// Begin/BeginTx 只返回空事务：DryRun 下 gorm 仍会走默认事务外壳，但不会真正下发语句。
func (noopConn) Begin() (driver.Tx, error) { return noopTx{}, nil }
func (noopConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return noopTx{}, nil
}

type noopTx struct{}

func (noopTx) Commit() error   { return nil }
func (noopTx) Rollback() error { return nil }

var registerNoopDriverOnce sync.Once

// dryRunDB 返回一个只会生成 SQL、不会执行、也不连库的 *gorm.DB。
func dryRunDB(t *testing.T) (*gorm.DB, *sqlCapture) {
	t.Helper()

	registerNoopDriverOnce.Do(func() { sql.Register("hermes_dryrun_noop", noopDriver{}) })

	cap := &sqlCapture{}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "hermes_dryrun_noop",
		DSN:        "hermes-dry-run",
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               cap,
	})
	require.NoError(t, err, "DryRun 建库本身不得连库")
	return db, cap
}

// requireNoOnConflict 回归护栏：任何一条真正会发给 PG 的 SQL 都不许带 ON CONFLICT ——
// 这两张表有 INSERT 规则，PG 会直接报 SQLSTATE 0A000 让写入全部失败卡死高度。
func requireNoOnConflict(t *testing.T, sqls []string) {
	t.Helper()
	for i, s := range sqls {
		require.NotContains(t, strings.ToUpper(s), "ON CONFLICT",
			"第 %d 条 SQL 不得出现 ON CONFLICT：表上有 INSERT 规则，PG 直接报 SQLSTATE 0A000: %s", i+1, s)
	}
}

func pickInsertStatements(sqls []string) (out []string) {
	for _, s := range sqls {
		if strings.HasPrefix(s, "INSERT INTO") {
			out = append(out, s)
		}
	}
	return
}

// transferKeys 把一批转账压成 "epoch/cid/index" 字符串，便于断言过滤结果与顺序。
func transferKeys(items []*po.FEvmERC20Transfer) []string {
	out := make([]string, 0, len(items))
	for _, v := range items {
		if v == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, fmt.Sprintf("%d/%s/%d", v.Epoch, v.Cid, v.Index))
	}
	return out
}

func transfer(epoch int64, cid string, index int) *po.FEvmERC20Transfer {
	return &po.FEvmERC20Transfer{
		Epoch:      epoch,
		Cid:        cid,
		Index:      index,
		ContractId: "0xaaa",
		From:       "0x1",
		To:         "0x2",
		Amount:     decimal.NewFromInt(1),
	}
}

// ---------------------------------------------------------------- 纯函数守卫

// TestFilterExistingERC20Transfers 守卫的纯函数部分（不依赖数据库）：
// 只按唯一键三元组 (epoch, cid, index) 剔除库里已有的行，其余原样保留。
func TestFilterExistingERC20Transfers(t *testing.T) {
	tests := []struct {
		name     string
		existing []*po.FEvmERC20Transfer
		items    []*po.FEvmERC20Transfer
		want     []string
	}{
		{
			name:     "重复项被过滤",
			existing: []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 0), transfer(6312687, "bafyOne", 1)},
			items:    []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 0), transfer(6312687, "bafyOne", 1)},
			want:     []string{},
		},
		{
			name:     "非重复项全部保留",
			existing: []*po.FEvmERC20Transfer{transfer(6312687, "bafyOther", 0)},
			items:    []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 0), transfer(6312687, "bafyTwo", 0)},
			want:     []string{"6312687/bafyOne/0", "6312687/bafyTwo/0"},
		},
		{
			name:     "库里没有已存在行（空集合）时不丢数据",
			existing: nil,
			items:    []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 0)},
			want:     []string{"6312687/bafyOne/0"},
		},
		{
			name:     "空批次",
			existing: []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 0)},
			items:    nil,
			want:     []string{},
		},
		{
			name:     "同 epoch 同 cid 多 index：只跳过已存在的那个 index",
			existing: []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 1)},
			items: []*po.FEvmERC20Transfer{
				transfer(6312687, "bafyOne", 0),
				transfer(6312687, "bafyOne", 1),
				transfer(6312687, "bafyOne", 2),
			},
			want: []string{"6312687/bafyOne/0", "6312687/bafyOne/2"},
		},
		{
			name:     "epoch 也是键的一部分：不同 epoch 的同行不互相跳过",
			existing: []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 0)},
			items:    []*po.FEvmERC20Transfer{transfer(6312688, "bafyOne", 0), transfer(6312687, "bafyOne", 0)},
			want:     []string{"6312688/bafyOne/0"},
		},
		{
			name:     "index 不同即不同行",
			existing: []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 0)},
			items:    []*po.FEvmERC20Transfer{transfer(6312687, "bafyOne", 1)},
			want:     []string{"6312687/bafyOne/1"},
		},
		{
			name:     "nil 元素不进插入列表、nil existing 被忽略",
			existing: []*po.FEvmERC20Transfer{nil, transfer(6312687, "bafyOne", 0)},
			items:    []*po.FEvmERC20Transfer{nil, transfer(6312687, "bafyTwo", 0)},
			want:     []string{"6312687/bafyTwo/0"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := transferKeys(FilterExistingERC20Transfers(tt.existing, tt.items))
			require.Equal(t, tt.want, got)
		})
	}
}

// ------------------------------------------------------------------ SQL 护栏

// TestCreateERC20TransferBatchSQLHasNoOnConflict 回归护栏 + 守卫接线：
// 一次写入 = 一条守卫查询（按本批 epoch 取 (epoch,cid,index)）+ 一条批量插入，
// 且任何 SQL 都不许出现 ON CONFLICT。
func TestCreateERC20TransferBatchSQLHasNoOnConflict(t *testing.T) {

	db, cap := dryRunDB(t)
	d := NewERC20Dal(db)

	items := []*po.FEvmERC20Transfer{
		{Epoch: 6312687, Cid: "bafyTransferOne", ContractId: "0xaaa", From: "0x1", To: "0x2", Amount: decimal.NewFromInt(1), Index: 0},
		{Epoch: 6312687, Cid: "bafyTransferOne", ContractId: "0xaaa", From: "0x2", To: "0x3", Amount: decimal.NewFromInt(2), Index: 1},
	}

	require.NoError(t, d.CreateERC20TransferBatch(context.Background(), items))

	sqls := cap.All()
	requireNoOnConflict(t, sqls)
	require.Len(t, sqls, 2, "先查后跳：一条守卫查询 + 一条批量插入，不做别的")

	guard := sqls[0]
	t.Logf("守卫查询 SQL: %s", guard)
	require.True(t, strings.HasPrefix(guard, "SELECT"), "第一条必须是守卫查询: %s", guard)
	require.Contains(t, guard, "erc_20_transfers")
	for _, col := range []string{`"epoch"`, `"cid"`, `"index"`} {
		require.Contains(t, guard, col, "守卫必须取唯一键三元组的 %s 列", col)
	}
	require.Contains(t, guard, "6312687", "守卫必须按本批 epoch 过滤")

	insert := sqls[1]
	t.Logf("插入 SQL: %s", insert)
	require.True(t, strings.HasPrefix(insert, "INSERT INTO"), "必须是插入语句: %s", insert)
	require.Contains(t, insert, "erc_20_transfers")
	for _, col := range []string{`"cid"`, `"index"`} {
		require.Contains(t, insert, col, "插入列必须保留 %s（唯一键靠它认行）: %s", col, insert)
	}
	require.Contains(t, insert, "6312687")
}

// TestCreateERC20TransferBatchKeepsBatchSize100 守卫不得顺手改批量大小：250 行仍是 3 条插入。
func TestCreateERC20TransferBatchKeepsBatchSize100(t *testing.T) {

	db, cap := dryRunDB(t)
	d := NewERC20Dal(db)

	items := make([]*po.FEvmERC20Transfer, 0, 250)
	for i := 0; i < 250; i++ {
		items = append(items, transfer(6312687, fmt.Sprintf("bafy%d", i), 0))
	}

	require.NoError(t, d.CreateERC20TransferBatch(context.Background(), items))

	sqls := cap.All()
	requireNoOnConflict(t, sqls)
	require.Len(t, pickInsertStatements(sqls), 3, "250 行按 100 一批切成 3 条插入")
}

// TestCreateERC20TransferBatchEmptyItemsNoSQL 空批次既不查库也不插入。
func TestCreateERC20TransferBatchEmptyItemsNoSQL(t *testing.T) {

	db, cap := dryRunDB(t)
	d := NewERC20Dal(db)

	require.NoError(t, d.CreateERC20TransferBatch(context.Background(), nil))

	require.Empty(t, cap.All(), "空批次不得产生任何语句")
}

// TestCreateERC20SwapInfoBatchSQLHasNoOnConflict 同一张 dal 上的换手信息表也不许带 ON CONFLICT：
// 该表当前无 INSERT 规则，但一旦按分区表声明就同样会被 PG 拒绝（SQLSTATE 0A000）。
func TestCreateERC20SwapInfoBatchSQLHasNoOnConflict(t *testing.T) {

	db, cap := dryRunDB(t)
	d := NewERC20Dal(db)

	items := []*po.FEvmERC20SwapInfo{
		{Cid: "bafySwapOne", Epoch: 6312687, Action: "swap", AmountIn: decimal.NewFromInt(1), AmountOut: decimal.NewFromInt(2), Dex: "0xdex"},
	}

	require.NoError(t, d.CreateERC20SwapInfoBatch(context.Background(), items))

	sqls := cap.All()
	requireNoOnConflict(t, sqls)
	require.Len(t, sqls, 1)
	require.True(t, strings.HasPrefix(sqls[0], "INSERT INTO"), "必须是插入语句: %s", sqls[0])
	require.Contains(t, sqls[0], "erc20_swap_info")
}

// ------------------------------------------------------- 守卫接线的假驱动用例
//
// 上面 DryRun 的守卫查询拿不到任何行，所以只能证明「发了这条查询」；下面这个最简假驱动
// 直接回答守卫查询（返回预设的「库里已有行」），证明过滤真的生效：库里已有的三元组不再插入。

type fakeGuardExec struct {
	sql  string
	args []driver.Value
}

type fakeGuardFixture struct {
	existing [][]driver.Value // 守卫查询返回的行：epoch, cid, index
	queries  []string
	execs    []fakeGuardExec
	mu       sync.Mutex
}

func (f *fakeGuardFixture) recordQuery(sql string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, sql)
}

func (f *fakeGuardFixture) recordExec(sql string, args []driver.Value) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.execs = append(f.execs, fakeGuardExec{sql: sql, args: args})
}

func (f *fakeGuardFixture) AllQueries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.queries))
	copy(out, f.queries)
	return out
}

func (f *fakeGuardFixture) AllExecs() []fakeGuardExec {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeGuardExec, len(f.execs))
	copy(out, f.execs)
	return out
}

type fakeGuardDriver struct{}

var (
	registerGuardDriverOnce sync.Once
	guardFixtureMu          sync.Mutex
	guardFixture            *fakeGuardFixture
)

func (fakeGuardDriver) Open(string) (driver.Conn, error) {
	guardFixtureMu.Lock()
	defer guardFixtureMu.Unlock()
	return &fakeGuardConn{fixture: guardFixture}, nil
}

type fakeGuardConn struct{ fixture *fakeGuardFixture }

func (c *fakeGuardConn) Prepare(query string) (driver.Stmt, error) {
	return &fakeGuardStmt{query: query, fixture: c.fixture}, nil
}
func (c *fakeGuardConn) Close() error              { return nil }
func (c *fakeGuardConn) Begin() (driver.Tx, error) { return noopTx{}, nil }
func (c *fakeGuardConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return noopTx{}, nil
}

type fakeGuardStmt struct {
	query   string
	fixture *fakeGuardFixture
}

func (s *fakeGuardStmt) Close() error  { return nil }
func (s *fakeGuardStmt) NumInput() int { return -1 }

func (s *fakeGuardStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.fixture.recordExec(s.query, args)
	return driver.RowsAffected(1), nil
}

func (s *fakeGuardStmt) Query([]driver.Value) (driver.Rows, error) {
	s.fixture.recordQuery(s.query)
	return &fakeGuardRows{cols: []string{"epoch", "cid", "index"}, rows: s.fixture.existing}, nil
}

type fakeGuardRows struct {
	cols []string
	rows [][]driver.Value
	i    int
}

func (r *fakeGuardRows) Columns() []string { return r.cols }
func (r *fakeGuardRows) Close() error      { return nil }
func (r *fakeGuardRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}

// execSQLs 取假驱动记录下来的 Exec 语句文本（真实执行路径下值是 $n 占位，值在 args 里）。
func execSQLs(execs []fakeGuardExec) []string {
	out := make([]string, 0, len(execs))
	for _, e := range execs {
		out = append(out, e.sql)
	}
	return out
}

// guardDB 返回一个假驱动支撑的 *gorm.DB：守卫查询一律返回现有行 existing，其余语句只记录。
func guardDB(t *testing.T, existing [][]driver.Value) (*gorm.DB, *fakeGuardFixture) {
	t.Helper()

	registerGuardDriverOnce.Do(func() { sql.Register("hermes_guard_fake", fakeGuardDriver{}) })

	fixture := &fakeGuardFixture{existing: existing}
	guardFixtureMu.Lock()
	guardFixture = fixture
	guardFixtureMu.Unlock()

	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "hermes_guard_fake",
		DSN:        "hermes-guard",
	}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	return db, fixture
}

// TestCreateERC20TransferBatchSkipsExistingRows 库里已有全部三元组（重跑已写过的批次）：
// 守卫把它们全过滤掉，一条 INSERT 都不许发 —— 这正是重跑幂等的实现方式。
func TestCreateERC20TransferBatchSkipsExistingRows(t *testing.T) {

	db, f := guardDB(t, [][]driver.Value{
		{int64(6312687), "bafyOne", int64(0)},
		{int64(6312687), "bafyOne", int64(1)},
	})
	d := NewERC20Dal(db)

	require.NoError(t, d.CreateERC20TransferBatch(context.Background(), []*po.FEvmERC20Transfer{
		transfer(6312687, "bafyOne", 0),
		transfer(6312687, "bafyOne", 1),
	}))

	queries := f.AllQueries()
	require.Len(t, queries, 1, "必须先按 epoch 查一次已有行")
	require.Contains(t, queries[0], "erc_20_transfers")

	execs := f.AllExecs()
	requireNoOnConflict(t, execSQLs(execs))
	require.Empty(t, execs, "库里已有的三元组不得再插入（重跑幂等）")
}

// TestCreateERC20TransferBatchInsertsOnlyMissingRows 部分已在库：只插入缺的那些行，
// 已有的行不重复写、也不报错。
func TestCreateERC20TransferBatchInsertsOnlyMissingRows(t *testing.T) {

	db, f := guardDB(t, [][]driver.Value{
		{int64(6312687), "bafyKeep", int64(0)},
	})
	d := NewERC20Dal(db)

	require.NoError(t, d.CreateERC20TransferBatch(context.Background(), []*po.FEvmERC20Transfer{
		transfer(6312687, "bafyKeep", 0),
		transfer(6312687, "bafyNew", 0),
	}))

	execs := f.AllExecs()
	requireNoOnConflict(t, execSQLs(execs))
	require.Len(t, execs, 1, "只有缺的那一行会被写入")
	require.Contains(t, execs[0].sql, "erc_20_transfers")
	require.Len(t, execs[0].args, 11, "只应剩一行值（11 列）：不得把已在库的行一起带上")

	joined := fmt.Sprint(execs[0].args)
	require.Contains(t, joined, "bafyNew")
	require.NotContains(t, joined, "bafyKeep", "已在库的行不得重复插入")
}
