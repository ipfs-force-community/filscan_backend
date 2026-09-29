package browser

import (
	"context"
	"encoding/json"
	"fmt"
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
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			sum := map[string]int64{}
			for _, rw := range probeRewards {
				if rw.epoch >= start && rw.epoch < end {
					sum[rw.miner] += rw.blockCount
				}
			}
			var rows []map[string]interface{}
			for _, miner := range []string{"01111", "02222"} {
				if v, ok := sum[miner]; ok {
					rows = append(rows, map[string]interface{}{
						"_id": miner, "TotalWinCount": v, "TotalGasReward": "0",
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

	// 3) wincount：TotalWinCount 逐值相等（重复行不得放大）；TotalGasReward 无 PG 来源，恒 0
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
		if !pgWins[i].TotalGasReward.IsZero() {
			t.Errorf("wincount[%d].TotalGasReward 应恒 0（PG 无该列），得到 %s", i, pgWins[i].TotalGasReward.String())
		}
	}
}

type probeReward struct {
	epoch      int64
	miner      string
	reward     string
	blockCount int64
}

// 与 probe 库/ stub 一致的数据；miner_rewards 每行是该 (epoch,miner) 的聚合值。
var probeRewards = []probeReward{
	{1000, "01111", "4860000000000000000", 2},
	{1001, "01111", "2430000000000000000", 1},
	{1002, "02222", "2430000000000000000", 1},
	{1003, "01111", "7290000000000000000", 3}, // 右端点外
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
		`create table chain.miner_win_counts (epoch bigint, miner varchar, win_count bigint) partition by range (epoch)`,
		`create table chain.miner_win_counts_p0 partition of chain.miner_win_counts for values from (0) to (1000000)`,
		`create index miner_win_counts_epoch_miner_index on only chain.miner_win_counts using btree (epoch, miner)`,
		`create index miner_win_counts_miner_epoch_index on only chain.miner_win_counts using btree (miner, epoch)`,
	}
	for _, rw := range probeRewards {
		statements = append(statements, fmt.Sprintf(
			`insert into chain.miner_rewards (epoch, miner, reward, block_count, block_time) values (%d, 'f%s', %s, %d, '2021-01-01 00:00:00')`,
			rw.epoch, rw.miner, rw.reward, rw.blockCount))
		// 每行故意插 3 份完全相同的 win_count（模拟纯 INSERT 重跑同一高度）
		for i := 0; i < 3; i++ {
			statements = append(statements, fmt.Sprintf(
				`insert into chain.miner_win_counts (epoch, miner, win_count) values (%d, 'f%s', %d)`,
				rw.epoch, rw.miner, rw.blockCount))
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
