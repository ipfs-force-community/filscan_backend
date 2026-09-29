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

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// 一个「会说 postgres 方言、会真按 where 过滤、会按 group by 聚合」的假库
//
// 为什么不能只断言 SQL 字符串：本缺陷的实质是**区间语义**（左开右闭被写成左闭右开），
// 只比字符串看不出「结果集里混进了不该有的高度」。所以这里让桩按 SQL 里写出的 where
// 真过滤、真聚合，再断言返回的矿工/数值 —— 旧代码在这个桩下必然返回左端高度的行。
//
// 本文件的桩自带（不与 dal_task_reward_test.go 的桩共用），避免两处测试互相耦合。
// ---------------------------------------------------------------------------

// accEpochBound 匹配 `epoch >= $1` / `epoch <= $2` 这类比较，捕获 (运算符, 参数序号)
var accEpochBound = regexp.MustCompile(`epoch\s*(>=|<=|>|<)\s*\$(\d+)`)

// accFromTable 抓出 SQL 里查询的表名（本文件的桩只区分 chain.miner_rewards /
// miner_win_counts / miner_gas_fees 三张，够覆盖被测的三个方法）
var accFromTable = regexp.MustCompile(`from\s+chain\.(miner_rewards|miner_win_counts|miner_gas_fees)`)

// accFakeRow 假库里的一行（三张表的列的并集，按表取用）
type accFakeRow struct {
	Miner      string
	Reward     string
	BlockCount int64
	WinCount   int64
	PreAgg     string
	ProveAgg   string
	SectorGas  string
	WdPostGas  string
	SealGas    string
}

// accFakeDB 假库：高度 ⇒ 该高度的行
type accFakeDB struct {
	table   map[int64][]accFakeRow
	queries []string
	args    [][]driver.Value
}

func (r *accFakeDB) lastQuery() (string, []driver.Value) {
	if len(r.queries) == 0 {
		return "", nil
	}
	return r.queries[len(r.queries)-1], r.args[len(r.args)-1]
}

// accAgg 聚合中间态（sum(...) 的语义）
type accAgg struct {
	miner                                           string
	reward                                          decimal.Decimal
	blockCount, winCount                            int64
	preAgg, proveAgg, sectorGas, wdPostGas, sealGas decimal.Decimal
}

func (r *accFakeDB) evaluate(query string, args []driver.Value) ([]string, [][]driver.Value, error) {
	m := accFromTable.FindStringSubmatch(query)
	if m == nil {
		return nil, nil, fmt.Errorf("accStub: SQL 里没有可识别的 from chain.<表>：%s", query)
	}
	table := m[1]
	cols := accColumns(table)

	bounds := map[int]driver.Value{}
	for i, v := range args {
		bounds[i+1] = v // postgres 的 $n 是 1-based
	}
	conds := accEpochBound.FindAllStringSubmatch(query, -1)

	epochs := make([]int64, 0, len(r.table))
	for e := range r.table {
		epochs = append(epochs, e)
	}
	sort.Slice(epochs, func(i, j int) bool { return epochs[i] < epochs[j] })

	aggs := map[string]*accAgg{}
	var order []string
	for _, e := range epochs {
		ok := true
		for _, c := range conds {
			n, err := strconv.Atoi(c[2])
			if err != nil {
				return nil, nil, fmt.Errorf("accStub: 占位符序号非法 %q", c[2])
			}
			bound, isInt := bounds[n].(int64)
			if !isInt {
				return nil, nil, fmt.Errorf("accStub: 参数 $%d 不是 int64（%T）", n, bounds[n])
			}
			switch c[1] {
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
		if !ok {
			continue
		}
		for _, row := range r.table[e] {
			a, exists := aggs[row.Miner]
			if !exists {
				a = &accAgg{miner: row.Miner}
				aggs[row.Miner] = a
				order = append(order, row.Miner)
			}
			a.reward = a.reward.Add(accDec(row.Reward))
			a.blockCount += row.BlockCount
			a.winCount += row.WinCount
			a.preAgg = a.preAgg.Add(accDec(row.PreAgg))
			a.proveAgg = a.proveAgg.Add(accDec(row.ProveAgg))
			a.sectorGas = a.sectorGas.Add(accDec(row.SectorGas))
			a.wdPostGas = a.wdPostGas.Add(accDec(row.WdPostGas))
			a.sealGas = a.sealGas.Add(accDec(row.SealGas))
		}
	}

	sort.Strings(order)
	var out [][]driver.Value
	for _, miner := range order {
		a := aggs[miner]
		vals := make([]driver.Value, 0, len(cols))
		for _, c := range cols {
			switch c {
			case "miner":
				vals = append(vals, a.miner)
			case "reward":
				vals = append(vals, a.reward.String())
			case "block_count":
				vals = append(vals, a.blockCount)
			case "win_count":
				vals = append(vals, a.winCount)
			case "pre_agg":
				vals = append(vals, a.preAgg.String())
			case "prove_agg":
				vals = append(vals, a.proveAgg.String())
			case "sector_gas":
				vals = append(vals, a.sectorGas.String())
			case "wd_post_gas":
				vals = append(vals, a.wdPostGas.String())
			case "seal_gas":
				vals = append(vals, a.sealGas.String())
			default:
				return nil, nil, fmt.Errorf("accStub: 未知列 %q", c)
			}
		}
		out = append(out, vals)
	}
	return cols, out, nil
}

func accDec(s string) decimal.Decimal {
	if s == "" {
		return decimal.Decimal{}
	}
	return decimal.RequireFromString(s)
}

// accColumns 三张表在各自 DAL 的 select 里出现的列（顺序与 select 一致）
func accColumns(table string) []string {
	switch table {
	case "miner_rewards":
		return []string{"miner", "reward", "block_count"}
	case "miner_win_counts":
		return []string{"miner", "win_count"}
	case "miner_gas_fees":
		return []string{"miner", "pre_agg", "prove_agg", "sector_gas", "wd_post_gas", "seal_gas"}
	}
	panic("accStub: 未知表 " + table)
}

type accStubDriver struct{ db *accFakeDB }

func (d *accStubDriver) Open(string) (driver.Conn, error) { return &accStubConn{db: d.db}, nil }

type accStubConn struct{ db *accFakeDB }

func (c *accStubConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("accStub: Prepare 未实现")
}
func (c *accStubConn) Close() error              { return nil }
func (c *accStubConn) Begin() (driver.Tx, error) { return nil, errors.New("accStub: Begin 未实现") }

func (c *accStubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	values := make([]driver.Value, 0, len(args))
	for _, a := range args {
		values = append(values, a.Value)
	}
	c.db.queries = append(c.db.queries, query)
	c.db.args = append(c.db.args, values)
	cols, rows, err := c.db.evaluate(query, values)
	if err != nil {
		return nil, err
	}
	return &accStubRows{cols: cols, rows: rows}, nil
}

type accStubRows struct {
	cols []string
	rows [][]driver.Value
	i    int
}

func (r *accStubRows) Columns() []string { return r.cols }
func (r *accStubRows) Close() error      { return nil }
func (r *accStubRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}

var accStubSeq atomic.Int64

func openAccStubDB(t *testing.T, db *accFakeDB) *gorm.DB {
	t.Helper()
	name := fmt.Sprintf("filscan_acc_stub_%d", accStubSeq.Add(1))
	sql.Register(name, &accStubDriver{db: db})
	conn, err := gorm.Open(
		postgres.New(postgres.Config{DriverName: name, DSN: "stub"}),
		&gorm.Config{Logger: gormlogger.Discard},
	)
	require.NoError(t, err)
	return conn
}

// accBoundaryFakeDB 造一份「左端、右端、区间外两侧」都有的假库：
//
//	epoch 2880：区间**之外**（左端的前一格）—— 这里的数据被旧实现错当成区间内
//	epoch 2888：区间**之外**（右端的后一格）
//	epoch 2881：区间内（紧邻左端）
//	epoch 4320：区间右端（= LteEnd）—— 旧实现把它整格丢掉
func accBoundaryFakeDB() *accFakeDB {
	return &accFakeDB{table: map[int64][]accFakeRow{
		2880: {{Miner: "f0100", Reward: "1000", BlockCount: 10, WinCount: 7, PreAgg: "1", ProveAgg: "2", SectorGas: "3", WdPostGas: "4", SealGas: "5"}},
		2881: {{Miner: "f0100", Reward: "11", BlockCount: 1, WinCount: 1, PreAgg: "10", ProveAgg: "20", SectorGas: "30", WdPostGas: "40", SealGas: "50"}},
		4320: {{Miner: "f0100", Reward: "22", BlockCount: 2, WinCount: 2, PreAgg: "100", ProveAgg: "200", SectorGas: "300", WdPostGas: "400", SealGas: "500"}},
		4328: {{Miner: "f0100", Reward: "3000", BlockCount: 30, WinCount: 3, PreAgg: "7", ProveAgg: "8", SectorGas: "9", WdPostGas: "10", SealGas: "11"}},
	}}
}

// 回归测试：GetMinersAccRewards 的入参是 chain.LORCRange（**左开右闭**，见
// pkg/chain/epoch.go:161-176「左开右闭」），调用方 calc-miner-owner-task 传的是
// NewLORCRange(prevEpoch, epoch)，即「本窗口要统计的高度」。旧 SQL 写成
// `epoch >= GtBegin and epoch < LteEnd` ⇒ 两端各偏一格：**多算了左端 prevEpoch
// （属于上一个窗口）、漏算了右端 epoch（属于本窗口）**，产出直接写进
// chain.miner_stats.acc_reward / acc_block_count（线上节点详情卡「总奖励/总出块」）。
//
// 旧代码下本用例必红：返回的 reward 会是 1011（含 2880 的 1000，漏掉 4320 的 22）。
func TestGetMinersAccRewardsUsesLeftOpenRightClosedWindow(t *testing.T) {
	db := accBoundaryFakeDB()
	rewards, err := NewMinerTaskDal(openAccStubDB(t, db)).GetMinersAccRewards(
		context.Background(), chain.NewLORCRange(2880, 4320))
	require.NoError(t, err)
	require.Len(t, rewards, 1)

	// 窗口 (2880, 4320] 恰好覆盖 2881 与 4320 两格：11 + 22
	require.True(t, decimal.RequireFromString("33").Equal(rewards[0].Reward),
		"窗口 (2880,4320] 只应累计 2881 与 4320 两格，实际 %s", rewards[0].Reward)
	require.Equal(t, int64(3), rewards[0].BlockCount, "出块数应为 1+2")
	require.Equal(t, "f0100", rewards[0].Miner)

	query, args := db.lastQuery()
	require.Contains(t, query, "epoch > $1", "左端必须开区间")
	require.Contains(t, query, "epoch <= $2", "右端必须闭区间")
	require.NotContains(t, query, "epoch >= $1", "左端写成闭区间会多算 prevEpoch")
	require.NotContains(t, query, "epoch < $2", "右端写成开区间会漏算 epoch")
	require.Equal(t, []driver.Value{int64(2880), int64(4320)}, args, "参数就是 LORCRange 的两端")
}

// 同一处缺陷的第二个方法：GetMinersAccWinCount（写 chain.miner_stats.acc_win_count，
// 线上「总赢票」与胜率 wining_rate 的分母都取自它）
func TestGetMinersAccWinCountUsesLeftOpenRightClosedWindow(t *testing.T) {
	db := accBoundaryFakeDB()
	items, err := NewMinerTaskDal(openAccStubDB(t, db)).GetMinersAccWinCount(
		context.Background(), chain.NewLORCRange(2880, 4320))
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, int64(3), items[0].WinCount, "窗口 (2880,4320] 的赢票应为 1+2，旧实现会算成 7+1=8")

	query, _ := db.lastQuery()
	require.Contains(t, query, "epoch > $1")
	require.Contains(t, query, "epoch <= $2")
}

// 同一处缺陷的第三个方法：GetMinersAccGasFees（写 acc_seal_gas / acc_wd_post_gas）
func TestGetMinersAccGasFeesUsesLeftOpenRightClosedWindow(t *testing.T) {
	db := accBoundaryFakeDB()
	fees, err := NewMinerTaskDal(openAccStubDB(t, db)).GetMinersAccGasFees(
		context.Background(), chain.NewLORCRange(2880, 4320))
	require.NoError(t, err)
	require.Len(t, fees, 1)
	require.True(t, decimal.RequireFromString("550").Equal(fees[0].SealGas),
		"窗口 (2880,4320] 的 seal_gas 应为 50+500，实际 %s", fees[0].SealGas)
	require.True(t, decimal.RequireFromString("330").Equal(fees[0].SectorGas),
		"窗口 (2880,4320] 的 sector_gas 应为 30+300，实际 %s", fees[0].SectorGas)
	require.True(t, decimal.RequireFromString("440").Equal(fees[0].WdPostGas),
		"窗口 (2880,4320] 的 wd_post_gas 应为 40+400，实际 %s", fees[0].WdPostGas)

	query, _ := db.lastQuery()
	require.Contains(t, query, "epoch > $1")
	require.Contains(t, query, "epoch <= $2")
}

// 左开右闭的极端形态：GtBegin 与 LteEnd 只差 1 时，窗口里**只有右端那一格**。
// 旧实现会返回 2880 那格（正好完全错开）。
func TestGetMinersAccRewardsOneEpochWindowIsRightEndOnly(t *testing.T) {
	db := accBoundaryFakeDB()
	rewards, err := NewMinerTaskDal(openAccStubDB(t, db)).GetMinersAccRewards(
		context.Background(), chain.NewLORCRange(2880, 2881))
	require.NoError(t, err)
	require.Len(t, rewards, 1)
	require.True(t, decimal.RequireFromString("11").Equal(rewards[0].Reward),
		"窗口 (2880,2881] 只含 2881 这一格，实际 %s", rewards[0].Reward)
	require.Equal(t, int64(1), rewards[0].BlockCount)
}

// 窗口内一格数据都没有（例如该窗口整段是空洞）时返回空集且不报错
func TestGetMinersAccRewardsEmptyWindow(t *testing.T) {
	db := accBoundaryFakeDB()
	rewards, err := NewMinerTaskDal(openAccStubDB(t, db)).GetMinersAccRewards(
		context.Background(), chain.NewLORCRange(2880, 2880))
	require.NoError(t, err)
	require.Empty(t, rewards, "GtBegin == LteEnd 时左开右闭窗口为空")
}

// 多矿工：窗口两端各偏一格时，**错的是「哪些矿工被算进来」**，而不只是数值大小 ——
// 左端那格爆块的 f_left 会被错算进来，右端那格爆块的 f_right 会被漏掉。
func TestGetMinersAccRewardsMinersAttributionAcrossWindowBoundaries(t *testing.T) {
	db := &accFakeDB{table: map[int64][]accFakeRow{
		100: {{Miner: "f_left", Reward: "1000", BlockCount: 5}},
		101: {{Miner: "f_inside", Reward: "10", BlockCount: 5}},
		102: {{Miner: "f_right", Reward: "20", BlockCount: 5}},
	}}
	rewards, err := NewMinerTaskDal(openAccStubDB(t, db)).GetMinersAccRewards(
		context.Background(), chain.NewLORCRange(100, 102))
	require.NoError(t, err)

	got := map[string]decimal.Decimal{}
	for _, v := range rewards {
		got[v.Miner] = v.Reward
	}
	require.NotContains(t, got, "f_left", "左端 100 属于上一个窗口，不能算进 (100,102]")
	require.True(t, decimal.RequireFromString("20").Equal(got["f_right"]),
		"右端 102 属于本窗口，必须算进来")
	require.Equal(t, 2, len(got))
}
