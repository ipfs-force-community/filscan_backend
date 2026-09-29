package dal

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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

// ---------------------------------------------------------------------------
// chain.miner_stats / chain.owner_stats 写入幂等的用例（全程不连库、不依赖 PG 实例）
//
// 生产库实测：两张表的自然键 (epoch, interval, miner/owner) 上有**非唯一**索引，
// 同一自然键躺着多行（最老 slot epoch=2160 一行矿工/所有者被写了 3 遍），而矿工/
// 所有者累计页面是把这些行**相加**的 ⇒ 重复行被重复累加。这里的用例钉住三件事：
//
//  1. 旧实现（裸 CreateInBatches 追加）在同一 (epoch, interval) 上跑两次会**多出一倍
//     行** —— TestSaveMinerStatsRepeatedRunKeepsSingleRow /
//     TestSaveOwnerStatsRepeatedRunKeepsSingleRow 在把守卫摘掉后必红（见用例注释）。
//  2. 新实现重复处理只留一份行，且**值刷新为最新计算值**（重跑一段高度要覆盖错值）。
//  3. 守卫是「先删后插」且删除严格按自然键精确匹配：传部分矿工不会误删同 slot 的
//     其他人，历史遗留的重复行会在重跑时被收敛成一行。
//
// 桩库只认 SaveMinerStats/SaveOwnerStats 真正会下发的两种语句，语句形状一变就报错，
// 所以它同时是 SQL 形状的回归护栏。
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------- 造数据

func minerStatRow(epoch int64, interval, miner string, accReward int64) *po.MinerStat {
	return &po.MinerStat{
		Epoch:         epoch,
		Interval:      interval,
		Miner:         miner,
		AccReward:     decimal.NewFromInt(accReward),
		AccBlockCount: accReward,
	}
}

func ownerStatRow(epoch int64, interval, owner string, accReward int64) *po.OwnerStat {
	return &po.OwnerStat{
		Epoch:         epoch,
		Interval:      interval,
		Owner:         owner,
		AccReward:     decimal.NewFromInt(accReward),
		AccBlockCount: accReward,
	}
}

func minerStatRowKeys(rows []*po.MinerStat) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, fmt.Sprintf("%d/%s/%s", r.Epoch, r.Interval, r.Miner))
	}
	return out
}

func ownerStatRowKeys(rows []*po.OwnerStat) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		if r == nil {
			out = append(out, "<nil>")
			continue
		}
		out = append(out, fmt.Sprintf("%d/%s/%s", r.Epoch, r.Interval, r.Owner))
	}
	return out
}

// ---------------------------------------------------------------- 纯函数：SQL 生成 / 归组 / 去重

// TestStatsNaturalKeyDeleteSQL 钉住守卫下发的删除语句：表名、列名（interval 必须带引号，
// 它是 PG 的类型关键字）、以及 epoch = ? 这个常量条件（分区裁剪与索引命中都靠它）。
func TestStatsNaturalKeyDeleteSQL(t *testing.T) {
	t.Run("miner", func(t *testing.T) {
		sqlText, args := statsNaturalKeyDeleteSQL(minerStatsTable, minerStatsActorColumn, 2160, "24h", []string{"f01000", "f01001"})
		require.Equal(t,
			`delete from chain.miner_stats where epoch = ? and "interval" = ? and miner in (?,?)`,
			sqlText)
		require.Equal(t, []interface{}{int64(2160), "24h", "f01000", "f01001"}, args)
	})

	t.Run("owner", func(t *testing.T) {
		sqlText, args := statsNaturalKeyDeleteSQL(ownerStatsTable, ownerStatsActorColumn, 2160, "2880", []string{"f01000"})
		require.Equal(t,
			`delete from chain.owner_stats where epoch = ? and "interval" = ? and owner in (?)`,
			sqlText)
		require.Equal(t, []interface{}{int64(2160), "2880", "f01000"}, args)
	})
}

// TestGroupStatsNaturalKeys 按 (epoch, interval) 归组：同 slot 一条 delete 覆盖，跨 slot 分开；
// group 顺序按首次出现的位置、组内按输入顺序。
func TestGroupStatsNaturalKeys(t *testing.T) {
	keys := []statsNaturalKey{
		{Epoch: 2160, Interval: "24h", Actor: "f1"},
		{Epoch: 2160, Interval: "24h", Actor: "f2"},
		{Epoch: 2160, Interval: "2880", Actor: "f1"},
		{Epoch: 2160, Interval: "24h", Actor: "f3"},
	}

	groups := groupStatsNaturalKeys(keys)
	require.Len(t, groups, 2, "同一 epoch 的 24h 与 2880 是两个不同的 slot，不能合成一组")
	require.Equal(t, statsNaturalKeyGroup{Epoch: 2160, Interval: "24h", Actors: []string{"f1", "f2", "f3"}}, groups[0])
	require.Equal(t, statsNaturalKeyGroup{Epoch: 2160, Interval: "2880", Actors: []string{"f1"}}, groups[1])

	require.Empty(t, groupStatsNaturalKeys(nil))
}

// TestDedupeStatsByNaturalKey 同批内的重复键只保留**最后一次**出现的行（后算出来的覆盖先算的），
// nil 直接丢掉，且不改变剩余元素的相对顺序。
func TestDedupeStatsByNaturalKey(t *testing.T) {
	t.Run("同批同键保留最后一条", func(t *testing.T) {
		rows := []*po.MinerStat{
			minerStatRow(2160, "24h", "f1", 1),
			minerStatRow(2160, "24h", "f2", 2),
			minerStatRow(2160, "24h", "f1", 111), // 后写者胜
			minerStatRow(2160, "24h", "f3", 3),
		}
		got := dedupeStatsByNaturalKey(rows, minerStatNaturalKey)
		require.Equal(t, []string{"2160/24h/f2", "2160/24h/f1", "2160/24h/f3"}, minerStatRowKeys(got),
			"去重后按各键最后一次出现的位置保序")
		require.True(t, decimal.NewFromInt(111).Equal(got[1].AccReward), "f1 必须留下最后那条（111），实际 %s", got[1].AccReward)
	})

	t.Run("nil 被丢掉", func(t *testing.T) {
		got := dedupeStatsByNaturalKey([]*po.MinerStat{nil, minerStatRow(2160, "24h", "f1", 1), nil}, minerStatNaturalKey)
		require.Equal(t, []string{"2160/24h/f1"}, minerStatRowKeys(got))
	})

	t.Run("同 epoch 不同 interval 不互相去重", func(t *testing.T) {
		got := dedupeStatsByNaturalKey([]*po.MinerStat{
			minerStatRow(2160, "24h", "f1", 1),
			minerStatRow(2160, "2880", "f1", 2),
		}, minerStatNaturalKey)
		require.Equal(t, []string{"2160/24h/f1", "2160/2880/f1"}, minerStatRowKeys(got))
	})

	t.Run("owner 同键保留最后一条", func(t *testing.T) {
		got := dedupeStatsByNaturalKey([]*po.OwnerStat{
			ownerStatRow(2160, "24h", "f1", 1),
			ownerStatRow(2160, "24h", "f1", 222),
		}, ownerStatNaturalKey)
		require.Equal(t, []string{"2160/24h/f1"}, ownerStatRowKeys(got))
		require.True(t, decimal.NewFromInt(222).Equal(got[0].AccReward))
	})
}

// ---------------------------------------------------------------- 内存桩库（无真实 PG）

// statsIdemFakeKey 桩库行的自然键。
type statsIdemFakeKey struct {
	Table    string
	Epoch    int64
	Interval string
	Actor    string
}

type statsIdemFakeRow struct {
	key    statsIdemFakeKey
	values map[string]driver.Value
}

// statsIdemFakeStore 内存里的 chain.miner_stats / chain.owner_stats。
// 故意用**切片**存行（而不是按自然键的 map）：真实的表上没有唯一索引，重复行必须
// 能共存、能被数出来 —— 否则「跑两次会不会多出一倍行」根本验不出来。
type statsIdemFakeStore struct {
	mu             sync.Mutex
	rows           []statsIdemFakeRow
	sqls           []string
	failNextInsert bool
}

func newStatsIdemFakeStore() *statsIdemFakeStore {
	return &statsIdemFakeStore{}
}

func (s *statsIdemFakeStore) snapshot() []statsIdemFakeRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]statsIdemFakeRow, len(s.rows))
	copy(out, s.rows)
	return out
}

func (s *statsIdemFakeStore) restore(snap []statsIdemFakeRow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = snap
}

// statements 返回桩库收到过的语句（顺序）。
func (s *statsIdemFakeStore) statements() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, len(s.sqls))
	copy(out, s.sqls)
	return out
}

// count 返回某张表当前的行数（重复行会重复计数）。
func (s *statsIdemFakeStore) count(table string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, r := range s.rows {
		if r.key.Table == table {
			n++
		}
	}
	return n
}

// rowsOf 返回某张表的全部行（按插入顺序）。
func (s *statsIdemFakeStore) rowsOf(table string) []statsIdemFakeRow {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []statsIdemFakeRow
	for _, r := range s.rows {
		if r.key.Table == table {
			out = append(out, r)
		}
	}
	return out
}

// seed 直接落一行，用来模拟库里已经有的历史数据（含历史遗留的重复行）。
// 列名与真实表一致（miner/owner、acc_reward），便于直接断言值。
func (s *statsIdemFakeStore) seed(table string, epoch int64, interval, actor string, accReward int64) {
	actorColumn := "miner"
	if table == ownerStatsTable {
		actorColumn = "owner"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = append(s.rows, statsIdemFakeRow{
		key: statsIdemFakeKey{Table: table, Epoch: epoch, Interval: interval, Actor: actor},
		values: map[string]driver.Value{
			"epoch":      epoch,
			"interval":   interval,
			actorColumn:  actor,
			"acc_reward": accReward,
		},
	})
}

func (s *statsIdemFakeStore) failInsertOnce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNextInsert = true
}

// statsIdemDeletePattern 只接受守卫真正生成的那种删除语句（顺带是形状回归护栏）。
// 驱动拿到的是带 $N 占位符的原始 SQL，比对前先把占位符统一成 ?（见 exec）。
var statsIdemDeletePattern = regexp.MustCompile(`^delete from (\S+) where epoch = \? and "interval" = \? and (miner|owner) in \(((?:\?,?)+)\)$`)

var statsIdemInsertPattern = regexp.MustCompile(`(?i)^insert into\s+(\S+)\s*\(([^)]*)\)\s+values\s`)

var statsIdemPlaceholderPattern = regexp.MustCompile(`\$\d+`)

func statsIdemNormalizePlaceholders(sqlText string) string {
	return statsIdemPlaceholderPattern.ReplaceAllString(sqlText, "?")
}

func statsIdemDriverValueToInt64(v driver.Value) (int64, error) {
	switch n := v.(type) {
	case int64:
		return n, nil
	case int:
		return int64(n), nil
	case []byte:
		return strconv.ParseInt(string(n), 10, 64)
	case string:
		return strconv.ParseInt(n, 10, 64)
	default:
		return 0, fmt.Errorf("桩库无法把 %T(%v) 当 int64", v, v)
	}
}

func statsIdemDriverValueToString(v driver.Value) string {
	switch s := v.(type) {
	case string:
		return s
	case []byte:
		return string(s)
	default:
		return fmt.Sprint(v)
	}
}

func (s *statsIdemFakeStore) exec(sqlText string, args []driver.Value) (int64, error) {
	stmt := strings.TrimSpace(sqlText)
	normalized := statsIdemNormalizePlaceholders(stmt)

	s.mu.Lock()
	s.sqls = append(s.sqls, stmt)
	failInsert := s.failNextInsert
	if strings.HasPrefix(strings.ToUpper(normalized), "INSERT INTO") {
		s.failNextInsert = false // 只失败一次
	}
	s.mu.Unlock()

	switch {
	case strings.HasPrefix(normalized, "delete from"):
		return s.execDelete(normalized, args)
	case strings.HasPrefix(normalized, "INSERT INTO"):
		if failInsert {
			return 0, errors.New("桩库：注入的 INSERT 失败（用于验证 delete+insert 的原子性）")
		}
		return s.execInsert(normalized, args)
	default:
		return 0, fmt.Errorf("桩库不认识的语句（SQL 形状变了就该让用例变红）: %s", stmt)
	}
}

func (s *statsIdemFakeStore) execDelete(stmt string, args []driver.Value) (int64, error) {
	m := statsIdemDeletePattern.FindStringSubmatch(stmt)
	if m == nil {
		return 0, fmt.Errorf("桩库不认识的 delete 形状: %s", stmt)
	}
	table, actorCount := m[1], strings.Count(m[3], "?")
	if len(args) != actorCount+2 {
		return 0, fmt.Errorf("桩库：delete 参数个数 %d 与占位符 %d 不匹配", len(args), actorCount+2)
	}
	epoch, err := statsIdemDriverValueToInt64(args[0])
	if err != nil {
		return 0, err
	}
	interval := statsIdemDriverValueToString(args[1])
	actors := map[string]struct{}{}
	for _, v := range args[2:] {
		actors[statsIdemDriverValueToString(v)] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.rows[:0]
	var removed int64
	for _, r := range s.rows {
		_, actorHit := actors[r.key.Actor]
		if r.key.Table == table && r.key.Epoch == epoch && r.key.Interval == interval && actorHit {
			removed++
			continue
		}
		kept = append(kept, r)
	}
	s.rows = kept
	return removed, nil
}

func (s *statsIdemFakeStore) execInsert(stmt string, args []driver.Value) (int64, error) {
	m := statsIdemInsertPattern.FindStringSubmatch(stmt)
	if m == nil {
		return 0, fmt.Errorf("桩库不认识的 insert 形状: %s", stmt)
	}
	table := strings.ReplaceAll(m[1], `"`, "")
	cols := strings.Split(m[2], ",")
	for i := range cols {
		cols[i] = strings.Trim(strings.TrimSpace(cols[i]), `"`)
	}
	if len(cols) == 0 || len(args)%len(cols) != 0 {
		return 0, fmt.Errorf("桩库：insert 列数 %d 与参数个数 %d 不匹配", len(cols), len(args))
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rows := int64(0)
	for i := 0; i < len(args); i += len(cols) {
		values := make(map[string]driver.Value, len(cols))
		for j, col := range cols {
			values[col] = args[i+j]
		}
		epoch, err := statsIdemDriverValueToInt64(values["epoch"])
		if err != nil {
			return rows, err
		}
		actor := ""
		switch {
		case values["miner"] != nil:
			actor = statsIdemDriverValueToString(values["miner"])
		case values["owner"] != nil:
			actor = statsIdemDriverValueToString(values["owner"])
		default:
			return rows, errors.New("桩库：insert 里既没有 miner 也没有 owner")
		}
		s.rows = append(s.rows, statsIdemFakeRow{
			key: statsIdemFakeKey{
				Table:    table,
				Epoch:    epoch,
				Interval: statsIdemDriverValueToString(values["interval"]),
				Actor:    actor,
			},
			values: values,
		})
		rows++
	}
	return rows, nil
}

// --- 极简驱动：把上面两种语句接到内存桩库上，并支持事务快照/回滚 ---

var (
	statsIdemFakeOnce   sync.Once
	statsIdemFakeMu     sync.Mutex
	statsIdemFakeStores = map[string]*statsIdemFakeStore{}
)

type statsIdemFakeDriver struct{}

func (statsIdemFakeDriver) Open(dsn string) (driver.Conn, error) {
	statsIdemFakeMu.Lock()
	defer statsIdemFakeMu.Unlock()
	store, ok := statsIdemFakeStores[dsn]
	if !ok {
		return nil, fmt.Errorf("桩库未注册: %s", dsn)
	}
	return &statsIdemFakeConn{store: store}, nil
}

type statsIdemFakeConn struct {
	store *statsIdemFakeStore
}

func (c *statsIdemFakeConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("桩库只用 PrepareContext")
}

func (c *statsIdemFakeConn) PrepareContext(_ context.Context, query string) (driver.Stmt, error) {
	return &statsIdemFakeStmt{conn: c, query: query}, nil
}

func (c *statsIdemFakeConn) Close() error { return nil }

func (c *statsIdemFakeConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *statsIdemFakeConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &statsIdemFakeTx{store: c.store, snapshot: c.store.snapshot()}, nil
}

type statsIdemFakeTx struct {
	store    *statsIdemFakeStore
	snapshot []statsIdemFakeRow
}

func (tx *statsIdemFakeTx) Commit() error { return nil }

func (tx *statsIdemFakeTx) Rollback() error {
	tx.store.restore(tx.snapshot)
	return nil
}

type statsIdemFakeStmt struct {
	conn  *statsIdemFakeConn
	query string
}

func (s *statsIdemFakeStmt) Close() error  { return nil }
func (s *statsIdemFakeStmt) NumInput() int { return -1 }

func (s *statsIdemFakeStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.execArgs(args)
}

func (s *statsIdemFakeStmt) ExecContext(_ context.Context, args []driver.NamedValue) (driver.Result, error) {
	values := make([]driver.Value, 0, len(args))
	for _, a := range args {
		values = append(values, a.Value)
	}
	return s.execArgs(values)
}

func (s *statsIdemFakeStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, errors.New("桩库不支持查询")
}

func (s *statsIdemFakeStmt) QueryContext(context.Context, []driver.NamedValue) (driver.Rows, error) {
	return nil, errors.New("桩库不支持查询")
}

func (s *statsIdemFakeStmt) execArgs(args []driver.Value) (driver.Result, error) {
	n, err := s.conn.store.exec(s.query, args)
	if err != nil {
		return nil, err
	}
	return driver.RowsAffected(n), nil
}

// newStatsIdemFakeDB 建一个只连内存桩库的 *gorm.DB。
func newStatsIdemFakeDB(t *testing.T) (*gorm.DB, *statsIdemFakeStore) {
	t.Helper()

	statsIdemFakeOnce.Do(func() { sql.Register("statsidem_fake", statsIdemFakeDriver{}) })

	store := newStatsIdemFakeStore()
	dsn := "statsidem-fake-" + t.Name()
	statsIdemFakeMu.Lock()
	statsIdemFakeStores[dsn] = store
	statsIdemFakeMu.Unlock()

	sqldb, err := sql.Open("statsidem_fake", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqldb.Close() })

	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqldb, WithoutReturning: true}), &gorm.Config{
		DisableAutomaticPing: true,
		Logger:               gormlogger.Discard,
	})
	require.NoError(t, err)
	return db, store
}

// ---------------------------------------------------------------- 幂等行为（桩库）

// TestSaveMinerStatsRepeatedRunKeepsSingleRow 是本次修复的**核心回归护栏**：
// 同一 (epoch, interval) 的同一批矿工跑两次，库里只能有一份行。
//
// RED 验证（把守卫摘掉、恢复成裸 CreateInBatches）：
//
//	把 SaveMinerStats 里的 Transaction(delete + insert) 换回 tx.CreateInBatches(rows, 100)，
//	本用例立刻变红：行数 8 ≠ 4（第二次跑把 4 行又追加了一遍——这正是生产库
//	epoch=2160 上 732 行 / 244 个矿工 = 3 倍的成因）。
func TestSaveMinerStatsRepeatedRunKeepsSingleRow(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	batch := []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 1),
		minerStatRow(2160, "24h", "f01001", 2),
		minerStatRow(2160, "24h", "f01002", 3),
		minerStatRow(2160, "24h", "f01003", 4),
	}

	require.NoError(t, d.SaveMinerStats(context.Background(), batch))
	require.Equal(t, 4, store.count(minerStatsTable), "第一次写入 4 行")

	require.NoError(t, d.SaveMinerStats(context.Background(), batch))
	require.Equal(t, 4, store.count(minerStatsTable),
		"同一 (epoch, interval, miner) 重复处理不得再产生重复行（旧实现这里会是 8 行）")

	require.NoError(t, d.SaveMinerStats(context.Background(), batch))
	require.Equal(t, 4, store.count(minerStatsTable), "跑第三次仍然只有一行一份")

	// 每一行都必须只出现一次
	seen := map[string]int{}
	for _, r := range store.rowsOf(minerStatsTable) {
		seen[fmt.Sprintf("%d/%s/%s", r.key.Epoch, r.key.Interval, r.key.Actor)]++
	}
	for k, n := range seen {
		require.Equal(t, 1, n, "自然键 %s 应当只有一行，实际 %d 行", k, n)
	}
}

// TestSaveOwnerStatsRepeatedRunKeepsSingleRow：owner_stats 同样的核心护栏。
func TestSaveOwnerStatsRepeatedRunKeepsSingleRow(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	batch := []*po.OwnerStat{
		ownerStatRow(2160, "2880", "f01000", 1),
		ownerStatRow(2160, "2880", "f01001", 2),
		ownerStatRow(2160, "2880", "f01002", 3),
	}

	require.NoError(t, d.SaveOwnerStats(context.Background(), batch))
	require.Equal(t, 3, store.count(ownerStatsTable))

	require.NoError(t, d.SaveOwnerStats(context.Background(), batch))
	require.Equal(t, 3, store.count(ownerStatsTable),
		"同一 (epoch, interval, owner) 重复处理不得再产生重复行（旧实现这里会是 6 行）")
}

// TestSaveMinerStatsReplayRefreshesValues 重跑必须把值**刷新**成最新计算值：
// 这正是选「先删后插」而不是「先查后跳」的原因（先查后跳会把旧的错值一直留在库里，
// 重跑一段修好的高度不收敛）。
func TestSaveMinerStatsReplayRefreshesValues(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 999), // 旧算法算出来的错值
	}))

	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 7), // 修好窗口后重算的值
	}))

	rows := store.rowsOf(minerStatsTable)
	require.Len(t, rows, 1, "重跑不得留下旧行")
	require.True(t, decimal.NewFromInt(7).Equal(decimal.RequireFromString(statsIdemFakeValue(rows[0], "acc_reward"))),
		"重跑后应当是新的值 7，实际 %v（先查后跳方案这里会留着 999）", rows[0].values["acc_reward"])
}

func statsIdemFakeValue(r statsIdemFakeRow, col string) string {
	if v, ok := r.values[col]; ok {
		return fmt.Sprint(v)
	}
	return ""
}

// TestSaveMinerStatsPartialBatchDoesNotTouchOtherMiners：守卫按自然键**精确删除**，
// 不是按 slot 整片删：只传部分矿工时，同 slot 里其他人的行必须原样留着。
func TestSaveMinerStatsPartialBatchDoesNotTouchOtherMiners(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	store.seed(minerStatsTable, 2160, "24h", "f01000", 1)
	store.seed(minerStatsTable, 2160, "24h", "f01001", 2)
	store.seed(minerStatsTable, 2160, "24h", "f01002", 3)

	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01001", 22),
	}))

	require.Equal(t, 3, store.count(minerStatsTable), "只覆盖传入的那个矿工，其他人不能被删")
	got := map[string]int64{}
	for _, r := range store.rowsOf(minerStatsTable) {
		got[r.key.Actor] = r.key.Epoch
	}
	require.Equal(t, int64(2160), got["f01000"])
	require.Equal(t, int64(2160), got["f01002"])
}

// TestSaveMinerStatsHealsLegacyDuplicateRows：库里已经有历史遗留的重复行（生产现状）
// 时，重跑一次就把同一自然键收敛成一行。
func TestSaveMinerStatsHealsLegacyDuplicateRows(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	for i := 0; i < 3; i++ { // epoch=2160 上「同一个矿工被写了 3 遍」
		store.seed(minerStatsTable, 2160, "24h", "f01000", int64(i))
	}
	require.Equal(t, 3, store.count(minerStatsTable))

	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 42),
	}))

	require.Equal(t, 1, store.count(minerStatsTable), "重跑后历史重复行必须被收敛成一行")
}

// TestSaveMinerStatsDedupeInBatchKeepsLastValue：同一批里自带重复键时，只写最后一条，
// 不会因为批内重复又多出一行。
func TestSaveMinerStatsDedupeInBatchKeepsLastValue(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 1),
		minerStatRow(2160, "24h", "f01000", 2),
		minerStatRow(2160, "24h", "f01000", 3),
	}))

	rows := store.rowsOf(minerStatsTable)
	require.Len(t, rows, 1)
	require.True(t, decimal.NewFromInt(3).Equal(decimal.RequireFromString(statsIdemFakeValue(rows[0], "acc_reward"))),
		"批内重复键保留最后一条（3），实际 %v", rows[0].values["acc_reward"])
}

// TestSaveMinerStatsAtomicOnInsertFailure：delete 与 insert 在同一事务里——
// insert 失败时不能把这一 slot 的行删掉就完事（否则重跑前页面直接缺数）。
func TestSaveMinerStatsAtomicOnInsertFailure(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	store.seed(minerStatsTable, 2160, "24h", "f01000", 1)
	store.failInsertOnce()

	err := d.SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 2),
	})
	require.Error(t, err, "insert 失败必须把错误抛出来")
	require.Equal(t, 1, store.count(minerStatsTable), "事务回滚：原来的那行必须还在")
	require.Equal(t, "1", statsIdemFakeValue(store.rowsOf(minerStatsTable)[0], "acc_reward"), "回滚后仍是旧值")

	// 失败之后重跑一次应当成功且只有一行
	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 2),
	}))
	require.Equal(t, 1, store.count(minerStatsTable))
	require.Equal(t, "2", statsIdemFakeValue(store.rowsOf(minerStatsTable)[0], "acc_reward"))
}

// TestSaveStatsEmptyBatchWritesNothing 空批 / 全 nil 批：一条语句都不发。
func TestSaveStatsEmptyBatchWritesNothing(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	require.NoError(t, d.SaveMinerStats(context.Background(), nil))
	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{}))
	require.NoError(t, d.SaveMinerStats(context.Background(), []*po.MinerStat{nil, nil}))
	require.NoError(t, d.SaveOwnerStats(context.Background(), nil))
	require.NoError(t, d.SaveOwnerStats(context.Background(), []*po.OwnerStat{nil}))
	require.Empty(t, store.statements(), "空批不该下发任何语句")
}

// TestSaveMinerStatsChunksDeleteByNaturalKey 一个 slot 超过 100 个矿工（真实库一天 244 个）
// 时按 100 分块下发 delete，且**所有 delete 都在 insert 之前**（先删后插）。
func TestSaveMinerStatsChunksDeleteByNaturalKey(t *testing.T) {
	db, store := newStatsIdemFakeDB(t)
	d := NewMinerTaskDal(db)

	batch := make([]*po.MinerStat, 0, 205)
	for i := 0; i < 205; i++ {
		batch = append(batch, minerStatRow(2160, "24h", fmt.Sprintf("f%05d", i), int64(i)))
	}

	require.NoError(t, d.SaveMinerStats(context.Background(), batch))
	require.Equal(t, 205, store.count(minerStatsTable))

	stmts := store.statements()
	deletes, inserts, lastDelete := 0, 0, -1
	for i, s := range stmts {
		switch {
		case strings.HasPrefix(s, "delete from"):
			deletes++
			lastDelete = i
		case strings.HasPrefix(s, "INSERT INTO"):
			inserts++
		default:
			t.Fatalf("桩库收到不认识的语句: %s", s)
		}
	}
	require.Equal(t, 3, deletes, "205 个矿工按 100 分块 ⇒ 3 条 delete")
	require.Equal(t, 3, inserts, "205 行按 100 分块 ⇒ 3 条 insert")
	for i, s := range stmts {
		if strings.HasPrefix(s, "INSERT INTO") {
			require.Greater(t, i, lastDelete, "所有 delete 必须先于 insert（先删后插）: %v", stmts)
			break
		}
	}

	// 再跑一次，块边界上也不能多行（205 不是 100 的整数倍）
	require.NoError(t, d.SaveMinerStats(context.Background(), batch))
	require.Equal(t, 205, store.count(minerStatsTable))
}

// ---------------------------------------------------------------- 真正会发给 PG 的 SQL（DryRun）

// statsIdemSQLCapture 只把 gorm 生成的 SQL 记下来（实现 gormlogger.Interface）。
type statsIdemSQLCapture struct {
	mu   sync.Mutex
	sqls []string
}

func (c *statsIdemSQLCapture) LogMode(gormlogger.LogLevel) gormlogger.Interface { return c }
func (c *statsIdemSQLCapture) Info(context.Context, string, ...interface{})     {}
func (c *statsIdemSQLCapture) Warn(context.Context, string, ...interface{})     {}
func (c *statsIdemSQLCapture) Error(context.Context, string, ...interface{})    {}

func (c *statsIdemSQLCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sqlText, _ := fc()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sqls = append(c.sqls, sqlText)
}

func (c *statsIdemSQLCapture) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, len(c.sqls))
	copy(out, c.sqls)
	return out
}

// statsIdemNoopDriver 空转驱动：DryRun 下 gorm 不会真正下发语句，这个驱动只用来
// 顶住「开事务」这类外壳调用；一旦真有语句被下发（Prepare）就直接报错。
type statsIdemNoopDriver struct{}

func (statsIdemNoopDriver) Open(string) (driver.Conn, error) { return statsIdemNoopConn{}, nil }

type statsIdemNoopConn struct{}

var errStatsIdemNoopTouched = errors.New("noop 驱动：DryRun 用例不应真正执行任何语句")

func (statsIdemNoopConn) Prepare(string) (driver.Stmt, error) { return nil, errStatsIdemNoopTouched }
func (statsIdemNoopConn) Close() error                        { return nil }
func (statsIdemNoopConn) Begin() (driver.Tx, error)           { return statsIdemNoopTx{}, nil }
func (statsIdemNoopConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return statsIdemNoopTx{}, nil
}

type statsIdemNoopTx struct{}

func (statsIdemNoopTx) Commit() error   { return nil }
func (statsIdemNoopTx) Rollback() error { return nil }

var statsIdemNoopOnce sync.Once

func statsIdemDryRunDB(t *testing.T) (*gorm.DB, *statsIdemSQLCapture) {
	t.Helper()

	statsIdemNoopOnce.Do(func() { sql.Register("statsidem_dryrun_noop", statsIdemNoopDriver{}) })

	capture := &statsIdemSQLCapture{}
	db, err := gorm.Open(postgres.New(postgres.Config{
		DriverName: "statsidem_dryrun_noop",
		DSN:        "statsidem-dry-run",
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
		Logger:               capture,
	})
	require.NoError(t, err, "DryRun 建库不得连库")
	return db, capture
}

func statsIdemCountPrefix(sqls []string, prefix string) int {
	n := 0
	for _, s := range sqls {
		if strings.HasPrefix(s, prefix) {
			n++
		}
	}
	return n
}

// TestSaveMinerStatsRealSQLShape 断言真正会发给 PG 的语句：一条按自然键的 delete，
// 后面跟着批量 insert；delete 必须带 epoch / "interval" / miner 三列，且绝不能出现
// ON CONFLICT（这两张表的分区/t 上有 INSERT 规则的历史教训见 feat/writer-idempotent）。
func TestSaveMinerStatsRealSQLShape(t *testing.T) {
	db, capture := statsIdemDryRunDB(t)

	require.NoError(t, NewMinerTaskDal(db).SaveMinerStats(context.Background(), []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 1),
		minerStatRow(2160, "24h", "f01001", 2),
	}))

	sqls := capture.all()
	require.Len(t, sqls, 2, "一批同 slot 的行 = 1 条 delete + 1 条 insert: %v", sqls)

	del := sqls[0]
	require.Contains(t, del, `delete from chain.miner_stats`)
	require.Contains(t, del, `epoch = 2160`)
	require.Contains(t, del, `"interval" = '24h'`)
	require.Contains(t, del, `miner in ('f01000','f01001')`)
	require.True(t, strings.HasPrefix(sqls[1], "INSERT INTO"), "delete 之后必须是 insert: %v", sqls)

	for _, s := range sqls {
		require.NotContains(t, strings.ToUpper(s), "ON CONFLICT",
			"绝不能再出现 ON CONFLICT：带 INSERT 规则的表上 PG 直接报 SQLSTATE 0A000: %s", s)
	}
}

// TestSaveOwnerStatsRealSQLShape owner_stats 同样：delete 认 owner 列。
func TestSaveOwnerStatsRealSQLShape(t *testing.T) {
	db, capture := statsIdemDryRunDB(t)

	require.NoError(t, NewMinerTaskDal(db).SaveOwnerStats(context.Background(), []*po.OwnerStat{
		ownerStatRow(2160, "2880", "f01000", 1),
	}))

	sqls := capture.all()
	require.Len(t, sqls, 2, "%v", sqls)
	require.Contains(t, sqls[0], `delete from chain.owner_stats where epoch = 2160 and "interval" = '2880' and owner in ('f01000')`)
	require.True(t, strings.HasPrefix(sqls[1], "INSERT INTO"))

	// 同一个 epoch 上的 24h 与 2880 各是一次独立调用，互不干扰（interval 必须在键里）
	require.NoError(t, NewMinerTaskDal(db).SaveOwnerStats(context.Background(), []*po.OwnerStat{
		ownerStatRow(2160, "24h", "f01000", 1),
	}))
	sqls = capture.all()
	require.Contains(t, sqls[2], `"interval" = '24h'`)
	require.Equal(t, 2, statsIdemCountPrefix(sqls, "delete from chain.owner_stats"))
}

// TestSaveStatsEmptyBatchSendsNoSQL DryRun 下空批也不该产生任何语句。
func TestSaveStatsEmptyBatchSendsNoSQL(t *testing.T) {
	db, capture := statsIdemDryRunDB(t)

	require.NoError(t, NewMinerTaskDal(db).SaveMinerStats(context.Background(), nil))
	require.NoError(t, NewMinerTaskDal(db).SaveOwnerStats(context.Background(), []*po.OwnerStat{}))
	require.Empty(t, capture.all())
}
