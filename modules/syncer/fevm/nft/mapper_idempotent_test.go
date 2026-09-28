package nft

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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

// 这些用例只验「真正会发给 PG 的 SQL」：用 gorm 的 DryRun 生成语句 + 捕获 logger.Trace，
// 完全不连库、不依赖 PG 实例。
//
// 背景：fevm.nft_transfers 没有唯一键，回放/同步重试会把整行重复写进去（实测一次分片
// 重跑产生 14,122 行重复）。SaveTransfers 里的应用层 cid 幂等只在「同一批里同 epoch
// 已存在」时有效，并发/重叠重放时两个 worker 可能都看不到对方，最终得靠库侧唯一约束兜底，
// 因此插入必须冲突即忽略。

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

// TestSaveTransfersUsesOnConflictDoNothing 每条 NFT 转账插入都必须带 ON CONFLICT DO NOTHING：
// 唯一键缺失时它是无副作用的未来防护，唯一键补齐后它就是重跑幂等的保证。
func TestSaveTransfersUsesOnConflictDoNothing(t *testing.T) {

	db, cap := dryRunDB(t)
	m := NewMapper(db)

	items := []*po.NFTTransfer{
		{Epoch: 6312687, Cid: "bafyNftOne", Contract: badContract, From: "0x1", To: "0x2", TokenId: tokenIdTopic, Method: "safeTransferFrom"},
		{Epoch: 6312687, Cid: "bafyNftTwo", Contract: goodContract, From: "0x1", To: "0x3", TokenId: tokenIdTopic},
	}

	require.NoError(t, m.SaveTransfers(context.Background(), items))

	sqls := cap.All()
	require.Contains(t, sqls[0], "SELECT", "应用层 cid 幂等逻辑必须原样保留（先查同 epoch 已有 cid）")

	inserts := insertStatements(sqls)
	require.Len(t, inserts, 2, "两条转账各自插入（逐行写入逻辑不变）")
	for i, insert := range inserts {
		t.Logf("第 %d 条插入 SQL: %s", i+1, insert)
		require.Contains(t, insert, "nft_transfers")
		require.Contains(t, insert, "ON CONFLICT DO NOTHING",
			"缺了它，重叠回放会重复写整行、并在补唯一键后硬报错卡死该高度: %s", insert)
		require.NotContains(t, insert, "ON CONFLICT (",
			"不得指定冲突列：冲突应交给库侧唯一约束兜底: %s", insert)
	}
}

// TestSaveTransfersEmptyItemsNoSQL 空批次不应产生任何插入（保持原有早退语义）。
func TestSaveTransfersEmptyItemsNoSQL(t *testing.T) {

	db, cap := dryRunDB(t)
	m := NewMapper(db)

	require.NoError(t, m.SaveTransfers(context.Background(), nil))

	require.Empty(t, insertStatements(cap.All()), "空批次不得产生插入语句")
}
