package evmtransfercmd

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 本文件是本包单测的假实现集合。全部**离线**：不连数据库、不连聚合器、不连适配器。
//
// 关键设计：fakeConn 把 gorm 的「数据库连接」换成一台只记录不下发的假连接 ——
// 于是「某段代码到底有没有向数据库写数据」变成一条可断言的 SQL 记录（记录为空 = 一条 SQL 都没发）。
// 这条判据比「数某个 repo 方法被调用了几次」更强：它能覆盖绕过 repo 直连 db 的写法。

// errFakeNoSQL 假连接对任何真实 SQL 的响应：一律失败（离线单测不允许下发任何语句）
var errFakeNoSQL = fmt.Errorf("fake conn: 离线单测不允许下发 SQL")

// fakeConn 实现 gorm.ConnPool + gorm.ConnPoolBeginner（*DB.Begin 会走 ConnPoolBeginner 分支），
// 再加 gorm.TxCommitter（*DB.Commit / Rollback 会做 TxCommitter 断言）：
// 提交/回滚与事务内的语句全部被记录，事务内语句同样被拒。
type fakeConn struct {
	mu        sync.Mutex
	sqls      []string
	txBegins  int
	commits   int
	rollbacks int
}

func (c *fakeConn) record(query string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = append(c.sqls, query)
}

// Statements 已下发（或被拒绝执行）的 SQL 列表
func (c *fakeConn) Statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sqls...)
}

// Reset 清空记录（用于「对照组先跑一遍再跑被测路径」）
func (c *fakeConn) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = nil
	c.txBegins, c.commits, c.rollbacks = 0, 0, 0
}

// TxCounts 返回 (开启事务数, 提交数, 回滚数)
func (c *fakeConn) TxCounts() (begins, commits, rollbacks int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.txBegins, c.commits, c.rollbacks
}

func (c *fakeConn) PrepareContext(_ context.Context, query string) (*sql.Stmt, error) {
	c.record(query)
	return nil, errFakeNoSQL
}

func (c *fakeConn) ExecContext(_ context.Context, query string, _ ...interface{}) (sql.Result, error) {
	c.record(query)
	return nil, errFakeNoSQL
}

func (c *fakeConn) QueryContext(_ context.Context, query string, _ ...interface{}) (*sql.Rows, error) {
	c.record(query)
	return nil, errFakeNoSQL
}

func (c *fakeConn) QueryRowContext(_ context.Context, query string, _ ...interface{}) *sql.Row {
	c.record(query)
	return nil
}

// BeginTx 返回一个「事务内连接」：它同样拒绝并记录语句，且支持 Commit/Rollback。
// 返回指针类型是必须的 —— gorm 的 Commit 会做 reflect.ValueOf(...).IsNil() 断言。
func (c *fakeConn) BeginTx(_ context.Context, _ *sql.TxOptions) (gorm.ConnPool, error) {
	c.mu.Lock()
	c.txBegins++
	c.mu.Unlock()
	return &fakeTx{parent: c}, nil
}

type fakeTx struct {
	parent *fakeConn
}

func (t *fakeTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.parent.PrepareContext(ctx, query)
}

func (t *fakeTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.parent.ExecContext(ctx, query, args...)
}

func (t *fakeTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.parent.QueryContext(ctx, query, args...)
}

func (t *fakeTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.parent.QueryRowContext(ctx, query, args...)
}

func (t *fakeTx) Commit() error {
	t.parent.mu.Lock()
	t.parent.commits++
	t.parent.mu.Unlock()
	return nil
}

func (t *fakeTx) Rollback() error {
	t.parent.mu.Lock()
	t.parent.rollbacks++
	t.parent.mu.Unlock()
	return nil
}

// newTestGormDB 用假连接构造 *gorm.DB：不拨号、不 ping（DisableAutomaticPing），
// 任何 SQL 都会落到 fakeConn 的记录里。
func newTestGormDB(t *testing.T, conn *fakeConn) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: conn}), &gorm.Config{
		Logger:               logger.Discard,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)
	return db
}

var _ londobell.Agg = (*fakeAgg)(nil)

// fakeAgg 聚合器假实现：只实现离线回放用到的 4 个方法，其余由嵌入接口兜底。
type fakeAgg struct {
	londobell.Agg

	headID int64 // LatestTipset 返回的 tipset ID（run() 会取 ID-1 当链头）

	mu     sync.Mutex
	epochs []int64 // Traces 被请求过的高度（按序，用于断言区间边界）
	traces func(epoch chain.Epoch) []*londobell.TraceMessage
	// statsReply 供 GetEvmTransferStats 之外的地方使用，这里留空
}

func newFakeAgg(headID int64, traces func(epoch chain.Epoch) []*londobell.TraceMessage) *fakeAgg {
	return &fakeAgg{headID: headID, traces: traces}
}

// EpochsRequested Traces 被请求过的高度（按序）
func (f *fakeAgg) EpochsRequested() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.epochs...)
}

func (f *fakeAgg) LatestTipset(_ context.Context) ([]*londobell.Tipset, error) {
	return []*londobell.Tipset{{ID: f.headID, Cids: []string{"head-cid"}}}, nil
}

func (f *fakeAgg) Tipset(_ context.Context, epoch chain.Epoch) ([]*londobell.Tipset, error) {
	return []*londobell.Tipset{{ID: epoch.Int64(), Cids: []string{fmt.Sprintf("cid-of-%d", epoch.Int64())}}}, nil
}

func (f *fakeAgg) ParentTipset(_ context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error) {
	return []*londobell.ParentTipset{{ID: start.Int64(), Cids: []string{fmt.Sprintf("parent-of-%d", start.Int64())}}}, nil
}

func (f *fakeAgg) Traces(_ context.Context, start, _ chain.Epoch) ([]*londobell.TraceMessage, error) {
	f.mu.Lock()
	f.epochs = append(f.epochs, start.Int64())
	fn := f.traces
	f.mu.Unlock()
	if fn == nil {
		return nil, nil
	}
	return fn(start), nil
}

var _ londobell.Adapter = (*fakeAdapter)(nil)

// fakeAdapter 适配器假实现：只实现 Actor（task 唯一的适配器依赖）。
type fakeAdapter struct {
	londobell.Adapter

	state   *londobell.ActorState
	errOnce error  // 首次调用返回的错误（模拟「该高度历史状态不可用」）
	onError func() // 返回错误那一刻的回调（用于在失败的瞬间快照写入计数）

	mu    sync.Mutex
	calls int
	errs  int
}

func (a *fakeAdapter) Actor(_ context.Context, _ chain.SmartAddress, _ *chain.Epoch) (*londobell.ActorState, error) {
	a.mu.Lock()
	a.calls++
	var err error
	if a.errOnce != nil {
		err = a.errOnce
		a.errOnce = nil
		a.errs++
	}
	state, hook := a.state, a.onError
	a.mu.Unlock()

	if err != nil {
		if hook != nil {
			hook()
		}
		return nil, err
	}
	return state, nil
}

// Calls 返回 (调用次数, 报错次数)
func (a *fakeAdapter) Calls() (calls, errs int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls, a.errs
}

var _ repository.EvmTransferRepo = (*fakeRepo)(nil)

// fakeRepo 派生表仓储假实现：写方法全部记账（用它断言「写入次数 = 0」），读方法返回预置数据。
type fakeRepo struct {
	repository.EvmTransferRepo

	mu            sync.Mutex
	transferSaves int
	transferRows  int
	statSaves     int
	statRows      int
	deleteCalls   int
	statsReads    int
	statsReply    []*bo.EVMTransferStats
}

func (f *fakeRepo) SaveEvmTransfers(_ context.Context, infos []*po.EvmTransfer) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transferSaves++
	f.transferRows += len(infos)
	return nil
}

func (f *fakeRepo) SaveEvmTransferStats(_ context.Context, infos []*po.EvmTransferStat) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statSaves++
	f.statRows += len(infos)
	return nil
}

func (f *fakeRepo) DeleteEvmTransfers(_ context.Context, _ chain.Epoch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
	return nil
}

func (f *fakeRepo) DeleteEvmTransferStats(_ context.Context, _ chain.Epoch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleteCalls++
	return nil
}

func (f *fakeRepo) GetEvmTransferStats(_ context.Context, _ chain.Epoch) ([]*bo.EVMTransferStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statsReads++
	return f.statsReply, nil
}

// Writes 写方法被调用的累计次数与行数（真写模式下会 > 0；--no-write 下必须恒为 0）
func (f *fakeRepo) Writes() (calls, rows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.transferSaves + f.statSaves + f.deleteCalls, f.transferRows + f.statRows
}

// TransferWrites 返回 (SaveEvmTransfers 调用次数, 行数)
func (f *fakeRepo) TransferWrites() (calls, rows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.transferSaves, f.transferRows
}

// StatWrites 返回 (SaveEvmTransferStats 调用次数, 行数)
func (f *fakeRepo) StatWrites() (calls, rows int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statSaves, f.statRows
}

// StatsReads GetEvmTransferStats 被调用次数（读透传的证据）
func (f *fakeRepo) StatsReads() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.statsReads
}

// fakeEvmTraces 构造 n 条「会被 EVMTransferTask 采纳」的 trace：
// Actor 以 /evm 结尾 + Method=InvokeContract + IsBlock=true（三个条件与 task 内的判据一致）。
func fakeEvmTraces(epoch chain.Epoch, n int) []*londobell.TraceMessage {
	traces := make([]*londobell.TraceMessage, 0, n)
	for i := 0; i < n; i++ {
		traces = append(traces, &londobell.TraceMessage{
			Cid:     fmt.Sprintf("bafy-msg-%d-%d", epoch.Int64(), i),
			Epoch:   epoch.Int64(),
			IsBlock: true,
			To:      chain.SmartAddress("f410fabcde"),
			From:    chain.SmartAddress("f1useraddress"),
			Value:   decimal.NewFromInt(int64(i + 1)),
			GasCost: &londobell.GasCost{TotalCost: decimal.NewFromInt(1000)},
			MsgRct:  &londobell.MsgRct{ExitCode: 0},
			Detail:  &londobell.MessageDetail{Actor: "f410fabcde/evm", Method: "InvokeContract"},
		})
	}
	return traces
}

// fakeActorState 假 Actor 状态
func fakeActorState() *londobell.ActorState {
	return &londobell.ActorState{
		ActorID:       "f0100",
		ActorAddr:     "f410fabcde",
		DelegatedAddr: "0x0000000000000000000000000000000000000abc",
		Balance:       decimal.NewFromInt(100),
	}
}

// runSyncerAndWait 跑同步器并等它自行退出（跑完 [start, end] 后 Run() 会返回，不会挂住）
func runSyncerAndWait(t *testing.T, run func(), timeout time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		run()
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("同步器未在 %s 内跑完（区间未走完或卡在重试）", timeout)
	}
}
