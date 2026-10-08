package dal

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// A1：统计页「累计区块奖励」曲线口径
//
// 曲线含义 = 累计发放给矿工的区块奖励（矿工实收），由 SQLBlockRewardTrend 从
// chain.builtin_actor_states.state(jsonb) 直接算出。三条路径：
//   ① v18 legacy：state.TotalStoragePowerReward；
//   ② v19 三计数器：TotalMintedReward − TotalBurnMinted − TotalExplicitMinted；
//   ③ 缺 v19 字段（补丁前/纯历史行）：回退 legacy。
//
// fixture 全部为链上真实抓取值（calibnet 升级高度 4109133 前后，见
// pkg/londobell/nv29_reward_test.go）。禁止编造数字。
// ---------------------------------------------------------------------------

const (
	// ① 当前 syncer 落库的 v18 行形态：TotalStoragePowerReward 有值，三个 v19 计数器为 "0"。
	//    注意 TotalMintedReward 是 "0" 而不是缺失 —— 这正是不能用 `is not null` 判断的原因。
	brtV18RealRow = `{"Epoch":4107459,"TotalStoragePowerReward":"110469517489432681170003963","TotalMintedReward":"0","TotalBurnMinted":"0","TotalExplicitMinted":"0"}`
	// ② v19 行：只有三个新计数器有值（TotalStoragePowerReward = "0"）。
	brtV19RealRow = `{"Epoch":4110339,"TotalStoragePowerReward":"0","TotalMintedReward":"110534789174400147324658484","TotalBurnMinted":"3338","TotalExplicitMinted":"1664003218449871124998"}`
	// ③ 补丁前（e21a4e0 之前）落库的历史行：结构体还没有三个新字段，state 里根本没有它们。
	brtLegacyOnlyRealRow = `{"Epoch":4107459,"ThisEpochReward":"23095629521969828224","TotalStoragePowerReward":"110469517489432681170003963"}`

	// 期望值（矿工实收，attoFIL）：
	brtWantV18        = "110469517489432681170003963" // TotalStoragePowerReward
	brtWantLegacyOnly = "110469517489432681170003963" // 同上
	brtWantV19        = "110533125171181697453530148" // 110534789174400147324658484 − 3338 − 1664003218449871124998
)

// TestBlockRewardTrendSQLSemantics 钉住 SQL 的口径与分支：
// 必须按 jsonb 读 TotalMintedReward/TotalBurnMinted/TotalExplicitMinted 相减，
// 以「数值非零」而非「非空」判断 v19（v18 行也带 "TotalMintedReward":"0"），
// 并保留 TotalStoragePowerReward 兜底；绝不能再用 b.balance / 1.1e9。
func TestBlockRewardTrendSQLSemantics(t *testing.T) {
	sql := normalizeSQL(SQLBlockRewardTrend)
	lower := strings.ToLower(sql)

	if strings.Contains(lower, "b.balance") || strings.Contains(lower, "1100000000") || strings.Contains(lower, "1.1e9") {
		t.Fatalf("曲线不得再取 f02 账户余额/1.1e9 基数（NV29 后失真）：\n%s", sql)
	}
	if !strings.Contains(lower, "(b.state ->> 'totalmintedreward')::numeric") {
		t.Fatalf("必须从 jsonb state 取 TotalMintedReward：\n%s", sql)
	}
	if !strings.Contains(lower, "coalesce((b.state ->> 'totalburnminted')::numeric, 0)") {
		t.Fatalf("必须扣减 TotalBurnMinted（缺失按 0）：\n%s", sql)
	}
	if !strings.Contains(lower, "coalesce((b.state ->> 'totalexplicitminted')::numeric, 0)") {
		t.Fatalf("必须扣减 TotalExplicitMinted（缺失按 0）：\n%s", sql)
	}
	if !strings.Contains(lower, "else coalesce((b.state ->> 'totalstoragepowerreward')::numeric, 0) end") {
		t.Fatalf("legacy 分支必须回退 TotalStoragePowerReward：\n%s", sql)
	}
	// 关键回归：v18 行也带 "TotalMintedReward":"0"（结构体无 omitempty），
	// 若用 `is not null` 判断就会把 v18 算成 0 —— 必须用数值非零。
	if strings.Contains(lower, "totalmintedreward') is not null") {
		t.Fatalf("不得用 `->> 'TotalMintedReward' is not null` 判断（v18 行的值是 \"0\"，会误判成 v19）：\n%s", sql)
	}
	if !strings.Contains(lower, "coalesce((b.state ->> 'totalmintedreward')::numeric, 0) <> 0") {
		t.Fatalf("v19 分支判断必须基于 TotalMintedReward 数值非零：\n%s", sql)
	}
}

// TestBlockRewardTrendThreePathsRealFixtures 用真实 state JSON 跑三条路径的口径断言。
// SQL 分支与该口径是同一条式子（SQL 直接内置），此用例钉住 fixtures 与期望值；
// 真正执行 SQL 的端到端验证见 TestBlockRewardTrendPGSemantics。
func TestBlockRewardTrendThreePathsRealFixtures(t *testing.T) {
	t.Run("v18_legacy", func(t *testing.T) {
		var s londobell.RewardActorState
		require.NoError(t, json.Unmarshal([]byte(brtV18RealRow), &s))
		require.True(t, s.TotalMintedReward.IsZero(), "前置：v18 行 TotalMintedReward 为 \"0\"（非缺失）")
		require.Equal(t, brtWantV18, s.MinerMinted().String(), "v18 矿工实收 = TotalStoragePowerReward")
	})

	t.Run("v19_three_counters", func(t *testing.T) {
		var s londobell.RewardActorState
		require.NoError(t, json.Unmarshal([]byte(brtV19RealRow), &s))
		require.Equal(t, brtWantV19, s.MinerMinted().String(), "v19 矿工实收 = minted − burn − explicit")
		// 不得把 TotalMintedReward 直接当矿工实收（会高估）。
		require.NotEqual(t, s.TotalMintedReward.String(), s.MinerMinted().String())
		want := s.TotalMintedReward.Sub(s.TotalBurnMinted).Sub(s.TotalExplicitMinted)
		require.True(t, want.Equal(s.MinerMinted()), "关系式：miner = minted − burn − explicit")
	})

	t.Run("v19_missing_fields_falls_back_legacy", func(t *testing.T) {
		var s londobell.RewardActorState
		require.NoError(t, json.Unmarshal([]byte(brtLegacyOnlyRealRow), &s))
		require.True(t, s.TotalMintedReward.IsZero() && s.TotalBurnMinted.IsZero() && s.TotalExplicitMinted.IsZero(),
			"前置：纯历史行没有 v19 三计数器")
		require.Equal(t, brtWantLegacyOnly, s.MinerMinted().String(), "缺字段回退 TotalStoragePowerReward")
	})
}

// ---------------------------------------------------------------------------
// 真实 postgres 端到端验证（默认跳过，只在给出 DSN 时跑）
//
// 跑法（临时库，跑完即 drop）：
//
//	createdb filscan_blockreward_e2e
//	FILSCAN_BLOCK_REWARD_TEST_DSN='host=/tmp port=5432 user=postgres dbname=filscan_blockreward_e2e sslmode=disable' \
//	    go test ./modules/common/infra/dal/ -run TestBlockRewardTrendPGSemantics -count=1 -v
//	dropdb filscan_blockreward_e2e
//
// 安全闸：本测试会 `drop schema chain cascade`，只允许跑在库名含 blockreward 的库上。
// ---------------------------------------------------------------------------

func brtPGConnect(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("FILSCAN_BLOCK_REWARD_TEST_DSN")
	if dsn == "" {
		t.Skipf("未设置 FILSCAN_BLOCK_REWARD_TEST_DSN，跳过真实 postgres 端到端验证（跑法见本文件头注释）")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)
	var name string
	require.NoError(t, db.Raw("select current_database()").Scan(&name).Error)
	require.Contains(t, strings.ToLower(name), "blockreward", "安全闸：只允许跑在库名含 blockreward 的临时库上，本次是 %q", name)
	return db
}

func brtPGSetup(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`drop schema if exists chain cascade`,
		`create schema chain`,
		`create table chain.builtin_actor_states (epoch bigint, actor varchar, state jsonb, balance numeric)`,
		`create table chain.miner_reward_stats (epoch bigint, interval varchar, acc_reward_per_t numeric)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error, s)
	}
	t.Cleanup(func() { require.NoError(t, db.Exec(`drop schema if exists chain cascade`).Error) })
}

// TestBlockRewardTrendPGSemantics 把三条路径的真实 fixture 写进 PG，跑真正的 DAL SQL，
// 断言取回的累计矿工实收：v18 → TotalStoragePowerReward；v19 → 相减；缺字段 → 回退 legacy。
func TestBlockRewardTrendPGSemantics(t *testing.T) {
	db := brtPGConnect(t)
	brtPGSetup(t, db)

	const actor = "f02" // builtin.RewardActorAddr.String()
	rows := []struct {
		epoch int64
		state string
	}{
		{4107459, brtV18RealRow},
		{4110339, brtV19RealRow},
		{4099999, brtLegacyOnlyRealRow},
	}
	for _, r := range rows {
		require.NoError(t, db.Exec(
			`insert into chain.builtin_actor_states (epoch, actor, state) values (?, ?, ?::jsonb)`,
			r.epoch, actor, r.state).Error)
		require.NoError(t, db.Exec(
			`insert into chain.miner_reward_stats (epoch, interval, acc_reward_per_t) values (?, '24h', 0)`,
			r.epoch).Error)
	}

	items, err := NewStatisticBlockRewardTrendBizDal(db).GetBlockRewardsByEpochs(
		context.Background(), "24h", []int64{4107459, 4110339, 4099999})
	require.NoError(t, err)
	require.Len(t, items, 3)

	got := map[int64]string{}
	for _, it := range items {
		got[it.Epoch] = it.AccBlockRewards.String()
	}
	require.Equal(t, brtWantV18, got[4107459], "v18 行必须走 legacy（TotalStoragePowerReward），不得因 TotalMintedReward=\"0\" 被算成 0")
	require.Equal(t, brtWantV19, got[4110339], "v19 行必须相减 minted − burn − explicit")
	require.Equal(t, brtWantLegacyOnly, got[4099999], "缺 v19 字段必须回退 legacy")
}
