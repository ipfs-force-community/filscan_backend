package dal

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
)

// 本文件全部离线：把 gorm 的「数据库连接」换成一台只记录、不下发的假连接，
// 于是「到底发了哪些 SQL、参数是什么、是不是同一个事务」都变成可断言的事实。
//
// 这里必须较真的一条：chain.large_transfers 的写入必须是**同一个事务里的 delete + insert**
// （崩溃/中断不能留下半截或重复行），所以断言不是「调用过某个方法」，而是 txBegins/commits/rollbacks
// 与「delete 在 insert 之前」的 SQL 顺序。

// fakeSQL 记录 SQL 与参数；failAll / failKeyword 用来制造失败。
type fakeSQL struct {
	mu       sync.Mutex
	stmts    []string
	args     [][]any
	txBegins int
	commits  int
	rollback int

	failAll     bool
	failKeyword string
}

func (f *fakeSQL) record(query string, args []any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stmts = append(f.stmts, query)
	f.args = append(f.args, args)
}

// counts 返回 (语句数, 事务 begin 数, commit 数, rollback 数)
func (f *fakeSQL) counts() (stmts, begins, commits, rollbacks int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.stmts), f.txBegins, f.commits, f.rollback
}

func (f *fakeSQL) statement(i int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.stmts[i]
}

func (f *fakeSQL) allArgs() [][]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]any(nil), f.args...)
}

func (f *fakeSQL) err(query string) error {
	if f.failAll || (f.failKeyword != "" && strings.Contains(query, f.failKeyword)) {
		return fmt.Errorf("fake pg: refused (%s)", query)
	}
	return nil
}

// PrepareContext / QueryContext / QueryRowContext 一律报错：本测试只关心写入路径，
// 若被测代码意外发起查询，会立刻以错误暴露出来而不是静默通过。
func (f *fakeSQL) PrepareContext(_ context.Context, query string) (*sql.Stmt, error) {
	f.record(query, nil)
	return nil, fmt.Errorf("fake pg: prepare not supported")
}

func (f *fakeSQL) ExecContext(_ context.Context, query string, args ...interface{}) (sql.Result, error) {
	f.record(query, args)
	if err := f.err(query); err != nil {
		return nil, err
	}
	return fakeResult(0), nil
}

func (f *fakeSQL) QueryContext(_ context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	f.record(query, args)
	return nil, fmt.Errorf("fake pg: query not supported")
}

func (f *fakeSQL) QueryRowContext(_ context.Context, query string, args ...interface{}) *sql.Row {
	f.record(query, args)
	return nil
}

// BeginTx 返回事务连接（必须是指针类型：gorm 的 Commit 会做 IsNil 断言）
func (f *fakeSQL) BeginTx(_ context.Context, _ *sql.TxOptions) (gorm.ConnPool, error) {
	f.mu.Lock()
	f.txBegins++
	f.mu.Unlock()
	return &fakeSQLTx{parent: f}, nil
}

// 注意：Commit / Rollback **只能**实现在事务连接（fakeSQLTx）上，不能实现在根连接上 ——
// 根连接一旦实现 gorm.TxCommitter，gorm 会把「根连接」当成「已经在事务里」，
// 于是走 SAVEPOINT 分支、既不 BEGIN 也不 COMMIT（本用例正是在断言真事务边界）。
func (t *fakeSQLTx) Commit() error {
	t.parent.mu.Lock()
	t.parent.commits++
	t.parent.mu.Unlock()
	return nil
}

func (t *fakeSQLTx) Rollback() error {
	t.parent.mu.Lock()
	t.parent.rollback++
	t.parent.mu.Unlock()
	return nil
}

type fakeSQLTx struct{ parent *fakeSQL }

func (t *fakeSQLTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.parent.PrepareContext(ctx, query)
}
func (t *fakeSQLTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.parent.ExecContext(ctx, query, args...)
}
func (t *fakeSQLTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.parent.QueryContext(ctx, query, args...)
}
func (t *fakeSQLTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.parent.QueryRowContext(ctx, query, args...)
}

type fakeResult int64

func (r fakeResult) LastInsertId() (int64, error) { return int64(r), nil }
func (r fakeResult) RowsAffected() (int64, error) { return int64(r), nil }

func newTestDal(t *testing.T) (*LargeTransferDal, *fakeSQL) {
	t.Helper()
	fake := &fakeSQL{}
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: fake}), &gorm.Config{
		Logger:               gormlogger.Discard,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)
	return NewLargeTransferDal(db), fake
}

func sampleRows() []*po.LargeTransfer {
	root := "bafy-root"
	return []*po.LargeTransfer{
		{Epoch: 6409152, Cid: "bafy-a", RootCid: &root, FromAddr: "1from", ToAddr: "3to",
			Value: "10000000000000000000000", Method: "InvokeContract", Depth: 2},
		{Epoch: 6409152, Cid: "bafy-b", RootCid: nil, FromAddr: "1from2", ToAddr: "3to2",
			Value: "20000000000000000000000", Method: "Send", Depth: 1},
	}
}

func TestReplaceLargeTransfersDeleteThenInsertInOneTx(t *testing.T) {
	d, fake := newTestDal(t)

	require.NoError(t, d.ReplaceLargeTransfers(context.Background(), 6409152, sampleRows()))

	_, begins, commits, rollbacks := fake.counts()
	require.Equal(t, 1, begins, "delete + insert 必须正好一个事务")
	require.Equal(t, 1, commits)
	require.Equal(t, 0, rollbacks)

	ins := insertStatements(fake)
	require.Len(t, ins, 1, "两条命中行合成一条批量 INSERT")
	require.Contains(t, ins[0], `"chain"."large_transfers"`)

	// 第一条是 delete（先删后插），参数是该高度
	require.Contains(t, fake.statement(0), "delete from chain.large_transfers where epoch = $1")
	require.Equal(t, []any{int64(6409152)}, fake.allArgs()[0])

	// 逐列逐值断言：列顺序 = DDL 顺序，value 是 attoFIL 十进制原文（绝不经过浮点），
	// root_cid 可空（无根 cid 的行是 NULL），地址是不带前缀的原文
	require.Equal(t, []string{
		"6409152", "bafy-a", "bafy-root", "1from", "3to", "10000000000000000000000", "InvokeContract", "2",
		"6409152", "bafy-b", "<nil>", "1from2", "3to2", "20000000000000000000000", "Send", "1",
	}, normalizeArgs(insertArgs(fake)))

	// 列名与线上口径一致
	for _, col := range []string{"epoch", "cid", "root_cid", "from_addr", "to_addr", "value", "method", "depth"} {
		require.Contains(t, ins[0], col)
	}
	joined := strings.ToLower(fmt.Sprint(insertArgs(fake)))
	require.NotContains(t, joined, "e+22", "金额不得出现浮点/科学计数法形态")
	require.NotContains(t, joined, "1e22")
}

// insertStatements 本次记录里所有 INSERT 语句
func insertStatements(f *fakeSQL) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, s := range f.stmts {
		if strings.Contains(strings.ToUpper(s), "INSERT INTO") {
			out = append(out, s)
		}
	}
	return out
}

// insertArgs 第一条 INSERT 的参数（gorm 会把多行摊平在一条语句里）
func insertArgs(f *fakeSQL) []any {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, s := range f.stmts {
		if strings.Contains(strings.ToUpper(s), "INSERT INTO") {
			return f.args[i]
		}
	}
	return nil
}

// normalizeArgs 把参数转成可比较的字符串：*string 解引用（database/sql 落库前也会解引用），nil 指针记 <nil>
func normalizeArgs(args []any) []string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if p, ok := a.(*string); ok {
			if p == nil {
				out = append(out, "<nil>")
				continue
			}
			out = append(out, *p)
			continue
		}
		out = append(out, fmt.Sprint(a))
	}
	return out
}

func TestReplaceLargeTransfersWithoutRowsStillDeletes(t *testing.T) {
	d, fake := newTestDal(t)

	require.NoError(t, d.ReplaceLargeTransfers(context.Background(), 6409152, nil))

	stmts, begins, commits, rollbacks := fake.counts()
	require.Equal(t, 1, stmts, "没有命中行时也要执行 delete（清掉该高度的陈旧行），但不发 insert")
	require.Contains(t, fake.statement(0), "delete from chain.large_transfers")
	require.Equal(t, 1, begins)
	require.Equal(t, 1, commits)
	require.Equal(t, 0, rollbacks)
	require.Empty(t, insertStatements(fake))
}

func TestReplaceLargeTransfersRollsBackOnFailure(t *testing.T) {
	t.Run("insert 失败：整个事务回滚", func(t *testing.T) {
		d, fake := newTestDal(t)
		fake.failKeyword = "INSERT INTO"

		err := d.ReplaceLargeTransfers(context.Background(), 6409152, sampleRows())
		require.Error(t, err)

		_, begins, commits, rollbacks := fake.counts()
		require.Equal(t, 1, begins)
		require.Equal(t, 0, commits)
		require.Equal(t, 1, rollbacks, "insert 失败必须回滚，不能留下「只删了没插」的半截状态")
		require.Equal(t, 1, len(insertStatements(fake)), "insert 被尝试过（因此失败是真实的写失败）")
		require.Contains(t, fake.statement(0), "delete from chain.large_transfers")
	})

	t.Run("delete 失败：不继续发 insert 且整体回滚", func(t *testing.T) {
		d, fake := newTestDal(t)
		fake.failAll = true

		err := d.ReplaceLargeTransfers(context.Background(), 6409152, sampleRows())
		require.Error(t, err)

		_, begins, commits, rollbacks := fake.counts()
		require.Equal(t, 1, begins)
		require.Equal(t, 0, commits)
		require.Equal(t, 1, rollbacks)
		require.Empty(t, insertStatements(fake), "delete 就失败了，不该继续发 insert")
	})
}

func TestDeleteLargeTransfersFromEpoch(t *testing.T) {
	d, fake := newTestDal(t)

	require.NoError(t, d.DeleteLargeTransfersFromEpoch(context.Background(), 6409152))

	stmts, begins, _, rollbacks := fake.counts()
	require.Equal(t, 1, stmts)
	require.Equal(t, 0, begins, "单条 delete 不需要显式事务")
	require.Equal(t, 0, rollbacks)
	require.Contains(t, fake.statement(0), "delete from chain.large_transfers")
	require.Contains(t, fake.statement(0), "epoch >= $1")
	require.Equal(t, []any{int64(6409152)}, fake.allArgs()[0])
}
