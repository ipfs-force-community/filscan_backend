package dal

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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

// 这些用例只验「真正会发给 PG 的 SQL」：用 gorm 的 DryRun 生成语句 + 捕获 logger.Trace，
// 完全不连库、不依赖 PG 实例。
//
// 为什么必须钉住 ON CONFLICT DO NOTHING：fevm.erc_20_transfers 在 PG 侧有
// UNIQUE (cid, "index")，回归成裸插入时重叠分片重跑会撞唯一键硬报错，
// 该高度任务失败并被框架无限重试（实测一次重跑产生 14,122 行重复）。

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

// TestCreateERC20TransferBatchUsesOnConflictDoNothing 转账表（有唯一键）必须冲突即忽略：
// 否则同一行在重叠重跑时被重复写入（实测 14,122 行重复），加了唯一键后更是直接报错卡死高度。
func TestCreateERC20TransferBatchUsesOnConflictDoNothing(t *testing.T) {

	db, cap := dryRunDB(t)
	d := NewERC20Dal(db)

	items := []*po.FEvmERC20Transfer{
		{Epoch: 6312687, Cid: "bafyTransferOne", ContractId: "0xaaa", From: "0x1", To: "0x2", Amount: decimal.NewFromInt(1), Index: 0},
		{Epoch: 6312687, Cid: "bafyTransferOne", ContractId: "0xaaa", From: "0x2", To: "0x3", Amount: decimal.NewFromInt(2), Index: 1},
	}

	require.NoError(t, d.CreateERC20TransferBatch(context.Background(), items))

	sqls := cap.All()
	require.Len(t, sqls, 1, "同一批仍只发一条批量插入（batchSize 100 不变）")

	insert := sqls[0]
	t.Logf("生成的 SQL: %s", insert)
	require.True(t, strings.HasPrefix(insert, "INSERT INTO"), "必须是插入语句: %s", insert)
	require.Contains(t, insert, "erc_20_transfers")
	require.Contains(t, insert, "ON CONFLICT DO NOTHING",
		"转账表有 UNIQUE (cid, \"index\")，重跑必须冲突即忽略而不是硬报错卡死该高度")
	require.NotContains(t, insert, "ON CONFLICT (",
		"不得指定冲突列：唯一键是 (cid, index)，指定错列会让冲突漏判")
}

// TestCreateERC20SwapInfoBatchUsesOnConflictDoNothing 该表当前没有唯一键，DO NOTHING 无副作用，
// 属未来防护：一旦补上唯一键，重叠重跑同样幂等。
func TestCreateERC20SwapInfoBatchUsesOnConflictDoNothing(t *testing.T) {

	db, cap := dryRunDB(t)
	d := NewERC20Dal(db)

	items := []*po.FEvmERC20SwapInfo{
		{Cid: "bafySwapOne", Epoch: 6312687, Action: "swap", AmountIn: decimal.NewFromInt(1), AmountOut: decimal.NewFromInt(2), Dex: "0xdex"},
	}

	require.NoError(t, d.CreateERC20SwapInfoBatch(context.Background(), items))

	sqls := cap.All()
	require.Len(t, sqls, 1)

	insert := sqls[0]
	require.True(t, strings.HasPrefix(insert, "INSERT INTO"), "必须是插入语句: %s", insert)
	require.Contains(t, insert, "erc20_swap_info")
	require.Contains(t, insert, "ON CONFLICT DO NOTHING")
}

// TestCreateERC20TransferBatchKeepsAllColumns 钉住 DO NOTHING 没有顺手改动写入列：
// cid / index 这两列必须照旧入库（唯一键就是靠它们认行的）。
func TestCreateERC20TransferBatchKeepsAllColumns(t *testing.T) {

	db, cap := dryRunDB(t)
	d := NewERC20Dal(db)

	require.NoError(t, d.CreateERC20TransferBatch(context.Background(), []*po.FEvmERC20Transfer{
		{Epoch: 6312687, Cid: "bafyOne", ContractId: "0xaaa", From: "0x1", To: "0x2", Amount: decimal.NewFromInt(7), Index: 3},
	}))

	insert := cap.All()[0]
	for _, col := range []string{`"cid"`, `"index"`} {
		require.Contains(t, insert, col, "插入列必须保留 %s: %s", col, insert)
	}
	require.Contains(t, insert, "6312687")
}
