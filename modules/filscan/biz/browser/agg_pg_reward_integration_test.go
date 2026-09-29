package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	londobellimpl "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell/impl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"

	"github.com/go-resty/resty/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 端到端集成测试（默认跳过）：把「真 PG 派生表」与「真聚合器客户端（打到本地 stub）」两条路径
// 各跑一次，逐字段断言相等。这是把「PG 读路径」从单测的假数据升级到真解析链的验证 ——
// 包括 decimal 的构造/字符串形态，以及 aggregator 客户端的 JSON 解包（TotalBlockReward 带引号字符串）。
//
// 跑法（一次性库，库名必须含 test/probe，防止误指线上）：
//
//	FILSCAN_PG_PARITY_DSN='host=127.0.0.1 port=5433 user=postgres dbname=filscan_probe sslmode=disable' \
//	  go test ./modules/filscan/biz/browser/ -run TestPgRewardAggMatchesAggregator -v
func TestPgRewardAggMatchesAggregator(t *testing.T) {
	dsn := os.Getenv("FILSCAN_PG_PARITY_DSN")
	if dsn == "" {
		t.Skip("未设置 FILSCAN_PG_PARITY_DSN，跳过（集成测试需要一次性 PG）")
	}
	if !strings.Contains(dsn, "test") && !strings.Contains(dsn, "probe") {
		t.Skipf("DSN 必须指向一次性库（库名含 test/probe），当前: %s", dsn)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接 PG 失败: %s", err)
	}
	if err := prepareProbeTables(db); err != nil {
		t.Fatalf("准备探针表失败: %s", err)
	}

	// 聚合器 stub：返回 londobell 线上响应形态（data 里的金额是 decimal128 的字符串形态）。
	srv := newWincountProbeAggServer(t)
	defer srv.Close()

	agg := londobellimpl.NewLondobellAggImpl(srv.URL, resty.New())
	conf := &config.Config{Feature: &config.Feature{
		MinerBlockRewardReadFromPg:  boolPtrLocal(true),
		MinersBlockRewardReadFromPg: boolPtrLocal(true),
		MinerWinCountReadFromPg:     boolPtrLocal(true),
	}}
	wrapped := NewPgRewardAgg(agg, db, conf)
	if _, ok := wrapped.(*pgRewardAgg); !ok {
		t.Fatalf("开关已开但 agg 未被装饰，得到 %T", wrapped)
	}

	ctx := context.Background()
	start, end := chain.Epoch(1000), chain.Epoch(1003)

	// 1) miner_blockreward：逐字段（epoch / 金额文本 / 出块数）
	aggBlocks, err := agg.MinerBlockReward(ctx, chain.SmartAddress("f01111"), types.Filters{Start: &start, End: &end})
	if err != nil {
		t.Fatalf("聚合器 miner_blockreward 失败: %s", err)
	}
	pgBlocks, err := wrapped.MinerBlockReward(ctx, chain.SmartAddress("f01111"), types.Filters{Start: &start, End: &end})
	if err != nil {
		t.Fatalf("PG miner_blockreward 失败: %s", err)
	}
	if len(aggBlocks) != len(pgBlocks) {
		t.Fatalf("miner_blockreward 行数不一致: 聚合器 %d vs PG %d", len(aggBlocks), len(pgBlocks))
	}
	for i := range aggBlocks {
		if aggBlocks[i].Id != pgBlocks[i].Id ||
			aggBlocks[i].TotalBlockReward.String() != pgBlocks[i].TotalBlockReward.String() ||
			aggBlocks[i].BlockCount != pgBlocks[i].BlockCount {
			t.Errorf("miner_blockreward[%d] 不一致: 聚合器 %+v(金额 %s) vs PG %+v(金额 %s)", i,
				aggBlocks[i], aggBlocks[i].TotalBlockReward.String(), pgBlocks[i], pgBlocks[i].TotalBlockReward.String())
		}
	}

	// 2) miners_blockreward：逐字段（epoch+miner / 金额文本 / 出块数）
	aggMiners, err := agg.MinersBlockReward(ctx, start, end)
	if err != nil {
		t.Fatalf("聚合器 miners_blockreward 失败: %s", err)
	}
	pgMiners, err := wrapped.MinersBlockReward(ctx, start, end)
	if err != nil {
		t.Fatalf("PG miners_blockreward 失败: %s", err)
	}
	if len(aggMiners) != len(pgMiners) {
		t.Fatalf("miners_blockreward 行数不一致: 聚合器 %d vs PG %d", len(aggMiners), len(pgMiners))
	}
	for i := range aggMiners {
		if aggMiners[i].Id != pgMiners[i].Id ||
			aggMiners[i].TotalBlockReward.String() != pgMiners[i].TotalBlockReward.String() ||
			aggMiners[i].BlockCount != pgMiners[i].BlockCount {
			t.Errorf("miners_blockreward[%d] 不一致: 聚合器 %+v vs PG %+v", i, aggMiners[i], pgMiners[i])
		}
	}

	// 3) wincount：TotalWinCount / TotalGasReward 逐值相等（重复行不得放大；gas_reward 逐值比对）
	aggWins, err := agg.WinCount(ctx, start, end)
	if err != nil {
		t.Fatalf("聚合器 wincount 失败: %s", err)
	}
	pgWins, err := wrapped.WinCount(ctx, start, end)
	if err != nil {
		t.Fatalf("PG wincount 失败: %s", err)
	}
	if len(aggWins) != len(pgWins) {
		t.Fatalf("wincount 行数不一致: 聚合器 %d vs PG %d", len(aggWins), len(pgWins))
	}
	for i := range aggWins {
		if chain.SmartAddress(aggWins[i].Id).Address() != chain.SmartAddress(pgWins[i].Id).Address() ||
			aggWins[i].TotalWinCount != pgWins[i].TotalWinCount {
			t.Errorf("wincount[%d] 不一致: 聚合器 %+v vs PG %+v", i, aggWins[i], pgWins[i])
		}
		if aggWins[i].TotalGasReward.String() != pgWins[i].TotalGasReward.String() {
			t.Errorf("wincount[%d].TotalGasReward 不一致: 聚合器 %q vs PG %q",
				i, aggWins[i].TotalGasReward.String(), pgWins[i].TotalGasReward.String())
		}
	}
}

// gas_reward 尚未回填（NULL）的区间必须**回落聚合器**而不是返回 0：
// TotalGasReward 的消费点 acl_block_chain.GetBlockDetails 拿它算 TxFeeReward / MinedReward。
// 这里直接把 probe 表里的 gas_reward 置回 NULL，模拟 migration/36 之后还没回填的历史分区。
func TestPgRewardWinCountFallsBackWhenGasRewardNotBackfilled(t *testing.T) {
	dsn := os.Getenv("FILSCAN_PG_PARITY_DSN")
	if dsn == "" {
		t.Skip("未设置 FILSCAN_PG_PARITY_DSN，跳过（集成测试需要一次性 PG）")
	}
	if !strings.Contains(dsn, "test") && !strings.Contains(dsn, "probe") {
		t.Skipf("DSN 必须指向一次性库（库名含 test/probe），当前: %s", dsn)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("连接 PG 失败: %s", err)
	}
	if err := prepareProbeTables(db); err != nil {
		t.Fatalf("准备探针表失败: %s", err)
	}
	// 只把 epoch 1001（f01111）那一行置回 NULL —— 同一请求区间内「部分回填」
	if err := db.Exec(`update chain.miner_win_counts set gas_reward = null where epoch = 1001`).Error; err != nil {
		t.Fatalf("置 NULL 失败: %s", err)
	}

	srv := newWincountProbeAggServer(t)
	defer srv.Close()

	agg := londobellimpl.NewLondobellAggImpl(srv.URL, resty.New())
	wrapped := NewPgRewardAgg(agg, db, &config.Config{Feature: &config.Feature{
		MinerWinCountReadFromPg: boolPtrLocal(true),
	}})

	rows, err := wrapped.WinCount(context.Background(), chain.Epoch(1000), chain.Epoch(1003))
	if err != nil {
		t.Fatalf("wincount 失败: %s", err)
	}
	// 回落 ⇒ 拿到的是聚合器侧的值（含非 0 的 gas reward），不是被 0 顶过的 PG 值
	if len(rows) != 2 {
		t.Fatalf("应回落聚合器拿到 2 行，得到 %d 行", len(rows))
	}
	for _, row := range rows {
		if row.Id == "01111" && row.TotalGasReward.String() != "89145023322864" {
			t.Errorf("未回填区间应回落聚合器（gasReward=89145023322864），得到 %q", row.TotalGasReward.String())
		}
	}
}

type probeReward struct {
	epoch      int64
	miner      string
	reward     string
	blockCount int64
	// gasReward 该 (epoch,miner) 的 gas_reward（attoFIL）。故意给不同的非零值 +
	// 一个 0：0 是聚合器的合法取值，必须和「未回填的 NULL」区分开。
	gasReward string
}

// 与 probe 库/ stub 一致的数据；miner_rewards 每行是该 (epoch,miner) 的聚合值。
var probeRewards = []probeReward{
	{1000, "01111", "4860000000000000000", 2, "89145023322864"},
	{1001, "01111", "2430000000000000000", 1, "0"},
	{1002, "02222", "2430000000000000000", 1, "214208455099239"},
	{1003, "01111", "7290000000000000000", 3, "1"}, // 右端点外
}

// prepareProbeTables 复刻 migration/1.chain.sql 的两张表（含「win_counts 索引非唯一且 on only」的现实），
// 并灌入带重复行的数据；每次调用重建，保证可重复。
func prepareProbeTables(db *gorm.DB) error {
	statements := []string{
		`create schema if not exists chain`,
		`drop table if exists chain.miner_rewards cascade`,
		`create table chain.miner_rewards (
			epoch bigint, miner varchar, reward numeric, block_count bigint, block_time timestamp,
			acc_reward numeric, acc_block_count numeric, prev_reward_ref bigint) partition by range (epoch)`,
		`create table chain.miner_rewards_p0 partition of chain.miner_rewards for values from (0) to (1000000)`,
		`create unique index miner_rewards_epoch_miner_uindex on chain.miner_rewards using btree (epoch, miner)`,
		`drop table if exists chain.miner_win_counts cascade`,
		// gas_reward 列 = migration/36.miner_win_counts_gas_reward.sql 的效果
		`create table chain.miner_win_counts (epoch bigint, miner varchar, win_count bigint, gas_reward numeric) partition by range (epoch)`,
		`create table chain.miner_win_counts_p0 partition of chain.miner_win_counts for values from (0) to (1000000)`,
		`create index miner_win_counts_epoch_miner_index on only chain.miner_win_counts using btree (epoch, miner)`,
		`create index miner_win_counts_miner_epoch_index on only chain.miner_win_counts using btree (miner, epoch)`,
	}
	for _, rw := range probeRewards {
		statements = append(statements, fmt.Sprintf(
			`insert into chain.miner_rewards (epoch, miner, reward, block_count, block_time) values (%d, 'f%s', %s, %d, '2021-01-01 00:00:00')`,
			rw.epoch, rw.miner, rw.reward, rw.blockCount))
		// 每行故意插 3 份完全相同的 win_count/gas_reward（模拟纯 INSERT 重跑同一高度）
		for i := 0; i < 3; i++ {
			statements = append(statements, fmt.Sprintf(
				`insert into chain.miner_win_counts (epoch, miner, win_count, gas_reward) values (%d, 'f%s', %d, %s)`,
				rw.epoch, rw.miner, rw.blockCount, rw.gasReward))
		}
	}
	for _, s := range statements {
		if err := db.Exec(strings.TrimSpace(s)).Error; err != nil {
			return fmt.Errorf("%s: %w", firstLine(s), err)
		}
	}
	return nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func boolPtrLocal(v bool) *bool { return &v }

// newWincountProbeAggServer 起一个聚合器 stub：按 probeRewards 复现三个端点的响应形态
// （金额是 decimal128 的字符串形态，_id 是**不带前缀**的 0… 地址）。两个集成测试共用。
func newWincountProbeAggServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Start *int64 `json:"start"`
			End   *int64 `json:"end"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		start, end := int64(0), int64(0)
		if req.Start != nil {
			start = *req.Start
		}
		if req.End != nil {
			end = *req.End
		}
		var data interface{}
		switch {
		case strings.HasSuffix(r.URL.Path, "/miner_blockreward"):
			var rows []map[string]interface{}
			for _, rw := range probeRewards {
				if rw.epoch >= start && rw.epoch < end && rw.miner == "01111" {
					rows = append(rows, map[string]interface{}{
						"_id": rw.epoch, "TotalBlockReward": rw.reward, "BlockCount": rw.blockCount,
					})
				}
			}
			data = rows
		case strings.HasSuffix(r.URL.Path, "/miners_blockreward"):
			var rows []map[string]interface{}
			for _, rw := range probeRewards {
				if rw.epoch >= start && rw.epoch < end {
					rows = append(rows, map[string]interface{}{
						"_id":              map[string]interface{}{"Epoch": rw.epoch, "Miner": rw.miner},
						"TotalBlockReward": rw.reward,
						"BlockCount":       rw.blockCount,
					})
				}
			}
			data = rows
		case strings.HasSuffix(r.URL.Path, "/wincount"):
			sumWin := map[string]int64{}
			sumGas := map[string]*big.Int{}
			for _, rw := range probeRewards {
				if rw.epoch >= start && rw.epoch < end {
					sumWin[rw.miner] += rw.blockCount
					g, ok := sumGas[rw.miner]
					if !ok {
						g = new(big.Int)
						sumGas[rw.miner] = g
					}
					v, _ := new(big.Int).SetString(rw.gasReward, 10)
					g.Add(g, v)
				}
			}
			var rows []map[string]interface{}
			for _, miner := range []string{"01111", "02222"} {
				if v, ok := sumWin[miner]; ok {
					rows = append(rows, map[string]interface{}{
						// 金额给 decimal128 的字符串形态（与线上一致）
						"_id": miner, "TotalWinCount": v, "TotalGasReward": sumGas[miner].String(),
					})
				}
			}
			data = rows
		default:
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": "0", "msg": "", "data": data})
	}))
}
