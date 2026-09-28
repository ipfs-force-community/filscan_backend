package offlinereplay

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 本文件是「离线回放」工具链共用的假实现。全部**离线**：不连数据库、不连聚合器、不连适配器。
//
// 关键设计：Recorder 把 gorm 的「数据库连接」换成一台只记录不下发的假连接 ——
// 于是「某段代码到底有没有向数据库写数据」变成一条可断言的 SQL 记录（记录为空 = 一条 SQL 都没发）。
// 这条判据比「数某个 repo 方法被调用了几次」更强：它能覆盖绕过 repo 直连 db 的写法，
// 也顺带证明同步指针 / 任务高度 / 跳过台账确实没被写（它们走的是另一个仓储）。

// errFakeNoSQL 假连接对任何真实 SQL 的响应：一律失败（离线单测不允许下发任何语句）
var errFakeNoSQL = fmt.Errorf("fake conn: 离线单测不允许下发 SQL")

// Recorder 实现 gorm.ConnPool + gorm.ConnPoolBeginner（*DB.Begin 走 ConnPoolBeginner 分支），
// 事务连接另外实现 gorm.TxCommitter（*DB.Commit / Rollback 会做 TxCommitter 断言）。
type Recorder struct {
	mu        sync.Mutex
	sqls      []string
	txBegins  int
	commits   int
	rollbacks int
}

func (c *Recorder) record(query string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = append(c.sqls, query)
}

// Statements 已下发（或被拒绝执行）的 SQL 列表
func (c *Recorder) Statements() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sqls...)
}

// Reset 清空记录（用于「对照组先跑一遍再跑被测路径」）
func (c *Recorder) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = nil
	c.txBegins, c.commits, c.rollbacks = 0, 0, 0
}

// TxCounts 返回 (开启事务数, 提交数, 回滚数)
func (c *Recorder) TxCounts() (begins, commits, rollbacks int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.txBegins, c.commits, c.rollbacks
}

func (c *Recorder) PrepareContext(_ context.Context, query string) (*sql.Stmt, error) {
	c.record(query)
	return nil, errFakeNoSQL
}

func (c *Recorder) ExecContext(_ context.Context, query string, _ ...interface{}) (sql.Result, error) {
	c.record(query)
	return nil, errFakeNoSQL
}

func (c *Recorder) QueryContext(_ context.Context, query string, _ ...interface{}) (*sql.Rows, error) {
	c.record(query)
	return nil, errFakeNoSQL
}

func (c *Recorder) QueryRowContext(_ context.Context, query string, _ ...interface{}) *sql.Row {
	c.record(query)
	return nil
}

// BeginTx 返回「事务内连接」：同样拒绝并记录语句，且支持 Commit/Rollback。
// 返回指针类型是必须的 —— gorm 的 Commit 会做 reflect.ValueOf(...).IsNil() 断言。
func (c *Recorder) BeginTx(_ context.Context, _ *sql.TxOptions) (gorm.ConnPool, error) {
	c.mu.Lock()
	c.txBegins++
	c.mu.Unlock()
	return &recorderTx{parent: c}, nil
}

type recorderTx struct {
	parent *Recorder
}

func (t *recorderTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.parent.PrepareContext(ctx, query)
}

func (t *recorderTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return t.parent.ExecContext(ctx, query, args...)
}

func (t *recorderTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return t.parent.QueryContext(ctx, query, args...)
}

func (t *recorderTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return t.parent.QueryRowContext(ctx, query, args...)
}

func (t *recorderTx) Commit() error {
	t.parent.mu.Lock()
	t.parent.commits++
	t.parent.mu.Unlock()
	return nil
}

func (t *recorderTx) Rollback() error {
	t.parent.mu.Lock()
	t.parent.rollbacks++
	t.parent.mu.Unlock()
	return nil
}

// OpenTestDB 用假连接构造 *gorm.DB：不拨号、不 ping（DisableAutomaticPing），
// 任何 SQL 都会落到 Recorder 的记录里。
func OpenTestDB(t *testing.T, rec *Recorder) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: rec}), &gorm.Config{
		Logger:               logger.Discard,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)
	return db
}

var _ londobell.Agg = (*FakeAgg)(nil)

// FakeAgg 聚合器假实现：只实现离线回放用到的 4 个方法，其余由嵌入接口兜底。
type FakeAgg struct {
	londobell.Agg

	// HeadID LatestTipset 返回的 tipset ID（run() 会取 ID-1 当链头）
	HeadID int64
	// TracesFn 按高度造 traces；nil 表示该高度没有 traces
	TracesFn func(epoch chain.Epoch) []*londobell.TraceMessage
	// CreateTimeFn 造 CreateTime 的返回；nil = 返回基时(0)且无错误。
	// actor 链路要用：CalcChangeActorTask.PrepareActor 只在「该 actor 不是新来者」时才问创建时间。
	CreateTimeFn func(addr chain.SmartAddress) (chain.Epoch, error)

	mu     sync.Mutex
	epochs []int64 // Traces 被请求过的高度（用于断言区间边界）
}

// NewFakeAgg 构造假聚合器
func NewFakeAgg(headID int64, tracesFn func(epoch chain.Epoch) []*londobell.TraceMessage) *FakeAgg {
	return &FakeAgg{HeadID: headID, TracesFn: tracesFn}
}

// EpochsRequested Traces 被请求过的高度（按序）
func (f *FakeAgg) EpochsRequested() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.epochs...)
}

// SortedEpochsRequested 升序版（高度是并发处理的，断言区间边界前先排序）
func (f *FakeAgg) SortedEpochsRequested() []int64 {
	got := f.EpochsRequested()
	for i := 1; i < len(got); i++ {
		for j := i; j > 0 && got[j] < got[j-1]; j-- {
			got[j], got[j-1] = got[j-1], got[j]
		}
	}
	return got
}

func (f *FakeAgg) LatestTipset(_ context.Context) ([]*londobell.Tipset, error) {
	return []*londobell.Tipset{{ID: f.HeadID, Cids: []string{"head-cid"}}}, nil
}

func (f *FakeAgg) Tipset(_ context.Context, epoch chain.Epoch) ([]*londobell.Tipset, error) {
	return []*londobell.Tipset{{ID: epoch.Int64(), Cids: []string{fmt.Sprintf("cid-of-%d", epoch.Int64())}}}, nil
}

func (f *FakeAgg) ParentTipset(_ context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error) {
	return []*londobell.ParentTipset{{ID: start.Int64(), Cids: []string{fmt.Sprintf("parent-of-%d", start.Int64())}}}, nil
}

// CreateTime 造 actor 创建时间（FakeAgg 对 actor 链路的最小支持）
func (f *FakeAgg) CreateTime(_ context.Context, addr chain.SmartAddress) (chain.Epoch, error) {
	if f.CreateTimeFn != nil {
		return f.CreateTimeFn(addr)
	}
	return 0, nil
}

func (f *FakeAgg) Traces(_ context.Context, start, _ chain.Epoch) ([]*londobell.TraceMessage, error) {
	f.mu.Lock()
	f.epochs = append(f.epochs, start.Int64())
	fn := f.TracesFn
	f.mu.Unlock()
	if fn == nil {
		return nil, nil
	}
	return fn(start), nil
}

var _ londobell.Adapter = (*FakeAdapter)(nil)

// FakeAdapter 适配器假实现（只实现 Actor / Epoch）。
type FakeAdapter struct {
	londobell.Adapter

	// State Actor 的返回；ErrOnce 首次调用返回的错误（模拟「该高度历史状态不可用」）；
	// OnError 返回错误那一刻的回调（用于在失败的瞬间快照写入计数）
	State   *londobell.ActorState
	ErrOnce error
	OnError func()
	// StateByActor 按 actor 地址返回不同状态（键命中时优先于 State）。
	// 需要「同一高度多个不同 actor」的链路用它 —— 如 chain.actor_actions 的 (epoch, actor_id) 主键。
	StateByActor map[chain.SmartAddress]*londobell.ActorState

	mu    sync.Mutex
	calls int
	errs  int
}

// Calls 返回 (调用次数, 报错次数)
func (a *FakeAdapter) Calls() (calls, errs int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls, a.errs
}

func (a *FakeAdapter) Actor(_ context.Context, actorId chain.SmartAddress, _ *chain.Epoch) (*londobell.ActorState, error) {
	a.mu.Lock()
	a.calls++
	var err error
	if a.ErrOnce != nil {
		err = a.ErrOnce
		a.ErrOnce = nil
		a.errs++
	}
	state, hook := a.State, a.OnError
	if s, ok := a.StateByActor[actorId]; ok {
		state = s
	}
	a.mu.Unlock()

	if err != nil {
		if hook != nil {
			hook()
		}
		return nil, err
	}
	return state, nil
}

func (a *FakeAdapter) Epoch(_ context.Context, epoch *chain.Epoch) (*londobell.EpochReply, error) {
	reply := &londobell.EpochReply{BlockCount: 1}
	if epoch != nil {
		reply.Epoch = epoch.Int64()
	}
	return reply, nil
}

// RunSyncerAndWait 跑同步器并等它自行退出（跑完 [start, end] 后 Run() 会返回，不会挂住）
func RunSyncerAndWait(t *testing.T, run func(), timeout time.Duration) {
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
