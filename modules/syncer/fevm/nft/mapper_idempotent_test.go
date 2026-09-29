package nft

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

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
)

// 这些用例只验「真正会发给 PG 的 SQL」，全程不连库、不依赖 PG 实例。
//
// 为什么 nft 写入只能是应用层「先查后跳」（cid）、绝不能出现 ON CONFLICT：
// fevm.nft_transfers 上有 INSERT 规则（range_insert_action_rule ⇒
// fevm.action_nft_transfers_range_insert，fevm_bak 里还有一份影子），PG 对带 INSERT/UPDATE
// 规则的表直接禁止 ON CONFLICT（SQLSTATE 0A000: INSERT with ON CONFLICT clause cannot be
// used with table that has INSERT or UPDATE rules）——实测加了它以后重跑已写过的 100 个高度
// 全红、框架无限重试。该表主键是 (epoch, cid)，所以认行的键就是 cid。
// TestSaveTransfersSQLHasNoOnConflict 就是防后来人再把 ON CONFLICT 加回来的回归护栏。

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

func insertStatements(sqls []string) (out []string) {
	for _, s := range sqls {
		if strings.HasPrefix(s, "INSERT INTO") {
			out = append(out, s)
		}
	}
	return
}

// requireNoOnConflict 回归护栏：任何一条真正会发给 PG 的 SQL 都不许带 ON CONFLICT ——
// nft_transfers 上有 INSERT 规则，PG 会直接报 SQLSTATE 0A000 让写入全部失败卡死高度。
func requireNoOnConflict(t *testing.T, sqls []string) {
	t.Helper()
	for i, s := range sqls {
		require.NotContains(t, strings.ToUpper(s), "ON CONFLICT",
			"第 %d 条 SQL 不得出现 ON CONFLICT：表上有 INSERT 规则，PG 直接报 SQLSTATE 0A000: %s", i+1, s)
	}
}

// TestSaveTransfersSQLHasNoOnConflict 回归护栏 + 守卫接线：
// 先按本批 epoch 查已有 cid（应用层先查后跳，原样保留），再逐行插入；
// 任何 SQL 都不许出现 ON CONFLICT。
func TestSaveTransfersSQLHasNoOnConflict(t *testing.T) {

	db, cap := dryRunDB(t)
	m := NewMapper(db)

	items := []*po.NFTTransfer{
		{Epoch: 6312687, Cid: "bafyNftOne", Contract: badContract, From: "0x1", To: "0x2", TokenId: tokenIdTopic, Method: "safeTransferFrom"},
		{Epoch: 6312687, Cid: "bafyNftTwo", Contract: goodContract, From: "0x1", To: "0x3", TokenId: tokenIdTopic},
	}

	require.NoError(t, m.SaveTransfers(context.Background(), items))

	sqls := cap.All()
	requireNoOnConflict(t, sqls)

	require.NotEmpty(t, sqls)
	require.Contains(t, sqls[0], "SELECT", "应用层 cid 先查后跳逻辑必须原样保留（先查同 epoch 已有 cid）")
	require.Contains(t, sqls[0], "nft_transfers")
	require.Contains(t, sqls[0], `"cid"`)
	require.Contains(t, sqls[0], "6312687", "守卫必须按本批 epoch 过滤")

	inserts := insertStatements(sqls)
	require.Len(t, inserts, 2, "两条转账各自插入（逐行写入逻辑不变）")
	for i, insert := range inserts {
		t.Logf("第 %d 条插入 SQL: %s", i+1, insert)
		require.Contains(t, insert, "nft_transfers")
		require.Contains(t, insert, `"cid"`)
		require.Contains(t, insert, "bafyNft")
	}
}

// TestSaveTransfersEmptyItemsNoSQL 空批次不应产生任何插入（保持原有早退语义）。
func TestSaveTransfersEmptyItemsNoSQL(t *testing.T) {

	db, cap := dryRunDB(t)
	m := NewMapper(db)

	require.NoError(t, m.SaveTransfers(context.Background(), nil))

	sqls := cap.All()
	requireNoOnConflict(t, sqls)
	require.Empty(t, insertStatements(sqls), "空批次不得产生插入语句")
}

// ------------------------------------------------------- 守卫接线的假驱动用例
//
// 上面 DryRun 的守卫查询拿不到任何行，只能证明「发了这条查询」；下面这个最简假驱动直接回答
// 守卫查询（返回预设的「库里已有 cid」），证明先查后跳确实生效：已有的 cid 不再插入。

type fakeGuardExec struct {
	sql  string
	args []driver.Value
}

type fakeGuardFixture struct {
	existingCids [][]driver.Value // 守卫查询返回的行：cid
	queries      []string
	execs        []fakeGuardExec
	mu           sync.Mutex
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
	return &fakeGuardRows{cols: []string{"cid"}, rows: s.fixture.existingCids}, nil
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

// guardDB 返回一个假驱动支撑的 *gorm.DB：守卫查询一律返回现有 cid，其余语句只记录。
func guardDB(t *testing.T, existingCids [][]driver.Value) (*gorm.DB, *fakeGuardFixture) {
	t.Helper()

	registerGuardDriverOnce.Do(func() { sql.Register("hermes_guard_fake_nft", fakeGuardDriver{}) })

	fixture := &fakeGuardFixture{existingCids: existingCids}
	guardFixtureMu.Lock()
	guardFixture = fixture
	guardFixtureMu.Unlock()

	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "hermes_guard_fake_nft",
		DSN:        "hermes-guard-nft",
	}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	return db, fixture
}

// TestSaveTransfersSkipsExistingCids 同 epoch 已有 cid（重叠回放）：
// 守卫把它们全部跳过，一条 INSERT 都不许发。
func TestSaveTransfersSkipsExistingCids(t *testing.T) {

	db, f := guardDB(t, [][]driver.Value{{"bafyNftOne"}, {"bafyNftTwo"}})
	m := NewMapper(db)

	require.NoError(t, m.SaveTransfers(context.Background(), []*po.NFTTransfer{
		{Epoch: 6312687, Cid: "bafyNftOne", Contract: goodContract, From: "0x1", To: "0x2", TokenId: tokenIdTopic},
		{Epoch: 6312687, Cid: "bafyNftTwo", Contract: goodContract, From: "0x1", To: "0x3", TokenId: tokenIdTopic},
	}))

	queries := f.AllQueries()
	require.Len(t, queries, 1, "必须先按 epoch 查一次已有 cid")
	require.Contains(t, queries[0], "nft_transfers")

	require.Empty(t, f.AllExecs(), "已有 cid 的转账不得再插入（重叠回放幂等）")
}

// TestSaveTransfersInsertsOnlyMissingCids 只有缺的那一行会被写，已有的 cid 不重复写。
func TestSaveTransfersInsertsOnlyMissingCids(t *testing.T) {

	db, f := guardDB(t, [][]driver.Value{{"bafyKeep"}})
	m := NewMapper(db)

	require.NoError(t, m.SaveTransfers(context.Background(), []*po.NFTTransfer{
		{Epoch: 6312687, Cid: "bafyKeep", Contract: goodContract, From: "0x1", To: "0x2", TokenId: tokenIdTopic},
		{Epoch: 6312687, Cid: "bafyNew", Contract: goodContract, From: "0x1", To: "0x3", TokenId: tokenIdTopic},
	}))

	execs := f.AllExecs()
	require.Len(t, execs, 1, "只有缺的那一行会被写入")

	sqls := make([]string, 0, len(execs))
	for _, e := range execs {
		sqls = append(sqls, e.sql)
	}
	requireNoOnConflict(t, sqls)
	require.Contains(t, sqls[0], "nft_transfers")

	joined := fmt.Sprint(execs[0].args)
	require.Contains(t, joined, "bafyNew")
	require.NotContains(t, joined, "bafyKeep", "已在库的行不得重复插入")
}
