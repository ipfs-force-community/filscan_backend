package dal

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

// ---------------------------------------------------------------------------
// 真实 postgres 端到端验证（默认跳过，只在给出 DSN 时跑）
//
// 跑法（**临时库**，跑完即 drop；绝不动生产库/生产表）：
//
//	createdb filscan_statsidem_e2e
//	FILSCAN_STATS_IDEM_TEST_DSN='host=/tmp port=5432 user=postgres dbname=filscan_statsidem_e2e sslmode=disable' \
//	    go test ./modules/common/infra/dal/ -run TestStatsIdemPG -count=1 -v
//	dropdb filscan_statsidem_e2e
//
// 安全闸：本测试会 `drop schema chain cascade`，因此**只允许跑在库名含 statsidem 的库上**，
// 连到别处直接 fail。测试自身在 t.Cleanup 里把新建的 chain schema 删掉，环境不留痕。
//
// 这里用**和生产一致的 DDL**（migration/1.chain.sql 的 chain.miner_stats /
// chain.owner_stats：partition by range (epoch) + 非唯一的自然键索引 + default 分区，
// 外加 26.owner_stat.sql 的 sector_power_change）在临时库里重建两张表，所以顺带验证
// 「新写入语句的列名与生产表完全对得上」。
// ---------------------------------------------------------------------------

const statsIdemPGDsnEnv = "FILSCAN_STATS_IDEM_TEST_DSN"

func statsIdemPGConnect(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := os.Getenv(statsIdemPGDsnEnv)
	if dsn == "" {
		t.Skipf("未设置 %s，跳过真实 postgres 端到端验证（跑法见本文件头注释）", statsIdemPGDsnEnv)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)

	var name string
	require.NoError(t, db.Raw("select current_database()").Scan(&name).Error)
	require.Contains(t, strings.ToLower(name), "statsidem",
		"安全闸：端到端测试会建/删 chain schema，只允许跑在库名含 statsidem 的临时库上，本次是 %q", name)

	t.Cleanup(func() {
		require.NoError(t, db.Exec(`drop schema if exists chain cascade`).Error)
	})
	return db
}

// statsIdemPGSetup 在临时库里重建与生产一致的 chain.miner_stats / chain.owner_stats。
func statsIdemPGSetup(t *testing.T, db *gorm.DB) {
	t.Helper()

	stmts := []string{
		`drop schema if exists chain cascade`,
		`create schema chain`,
		`create table chain.miner_stats
		(
			epoch                    bigint,
			miner                    varchar,
			interval                 varchar,
			prev_epoch_ref           bigint,
			raw_byte_power_change    numeric,
			quality_adj_power_change numeric,
			initial_pledge_change    numeric,
			acc_reward               numeric,
			acc_block_count          bigint,
			acc_block_count_percent  numeric,
			acc_win_count            bigint,
			acc_seal_gas             numeric,
			acc_wd_post_gas          numeric,
			acc_reward_percent       numeric,
			sector_count_change      bigint,
			reward_power_ratio       numeric,
			wining_rate              numeric,
			luck_rate                numeric
		) partition by RANGE (epoch)`,
		`create table chain.miner_stats_p0 partition of chain.miner_stats for values from (0) to (2880)`,
		`create table chain.miner_stats_p1 partition of chain.miner_stats for values from (2880) to (57600)`,
		`create table chain.miner_stats_pdefault partition of chain.miner_stats default`,
		`CREATE INDEX miner_stats_epoch_miner_interval_index ON chain.miner_stats USING btree (epoch, miner, "interval")`,

		`create table chain.owner_stats
		(
			epoch                    bigint,
			owner                    varchar,
			interval                 varchar,
			prev_epoch_ref           bigint,
			raw_byte_power_change    numeric,
			quality_adj_power_change numeric,
			initial_pledge_change    numeric,
			acc_reward               numeric,
			acc_block_count          bigint,
			acc_block_count_percent  numeric,
			acc_win_count            bigint,
			acc_seal_gas             numeric,
			acc_wd_post_gas          numeric,
			acc_reward_percent       numeric,
			sector_count_change      bigint,
			reward_power_ratio       numeric,
			sector_power_change      numeric
		) partition by RANGE (epoch)`,
		`create table chain.owner_stats_p0 partition of chain.owner_stats for values from (0) to (2880)`,
		`create table chain.owner_stats_p1 partition of chain.owner_stats for values from (2880) to (57600)`,
		`create table chain.owner_stats_pdefault partition of chain.owner_stats default`,
		`CREATE INDEX owner_stats_epoch_owner_interval_index ON chain.owner_stats USING btree (epoch, owner, "interval")`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error, s)
	}
}

// statsIdemPGCountRows 数某张表的总行数（重复行会重复计入）。
func statsIdemPGCountRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(fmt.Sprintf(`select count(*) from %s`, table)).Scan(&n).Error)
	return n
}

// statsIdemPGDuplicateKeys 数「同一自然键出现多行」的自然键个数（>0 就是有重复）。
func statsIdemPGDuplicateKeys(t *testing.T, db *gorm.DB, table, actorColumn string) int64 {
	t.Helper()
	var n int64
	require.NoError(t, db.Raw(fmt.Sprintf(`
		select count(*) from (
			select 1 from %s group by epoch, %s, "interval" having count(*) > 1
		) dup`, table, actorColumn)).Scan(&n).Error)
	return n
}

// TestStatsIdemPGRepeatedWriteNoDuplicate 核心端到端：生产同构的分区表上，
// 同一 (epoch, interval) 的同一批矿工/所有者跑三次，最多只有一份行，且值是最后一次算的。
func TestStatsIdemPGRepeatedWriteNoDuplicate(t *testing.T) {
	db := statsIdemPGConnect(t)
	statsIdemPGSetup(t, db)
	d := NewMinerTaskDal(db)
	ctx := context.Background()

	// ---- 矿工 ----
	miners := []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 1),
		minerStatRow(2160, "24h", "f01001", 2),
		minerStatRow(2160, "24h", "f01002", 3),
	}
	require.NoError(t, d.SaveMinerStats(ctx, miners))
	require.EqualValues(t, 3, statsIdemPGCountRows(t, db, minerStatsTable), "第一次写入 3 行，实际页面累加应当只有 1 份")

	require.NoError(t, d.SaveMinerStats(ctx, miners))
	require.EqualValues(t, 3, statsIdemPGCountRows(t, db, minerStatsTable),
		"同一 (epoch, interval, miner) 重复处理不能再多出行（旧实现这里是 6 行，页面上会被重复累加）")

	refreshed := []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 111),
		minerStatRow(2160, "24h", "f01001", 2),
		minerStatRow(2160, "24h", "f01002", 3),
	}
	require.NoError(t, d.SaveMinerStats(ctx, refreshed))
	require.EqualValues(t, 3, statsIdemPGCountRows(t, db, minerStatsTable))
	require.Zero(t, statsIdemPGDuplicateKeys(t, db, minerStatsTable, "miner"))

	var accReward float64
	require.NoError(t, db.Raw(`select acc_reward from chain.miner_stats where epoch = 2160 and miner = 'f01000' and "interval" = '24h'`).Scan(&accReward).Error)
	require.Equal(t, 111.0, accReward, "重跑必须把值刷新成最新计算值（先查后跳方案这里会留着旧值）")

	// ---- 所有者 ----
	owners := []*po.OwnerStat{
		ownerStatRow(2160, "2880", "f01000", 7),
		ownerStatRow(2160, "2880", "f01001", 8),
	}
	require.NoError(t, d.SaveOwnerStats(ctx, owners))
	require.NoError(t, d.SaveOwnerStats(ctx, owners))
	require.EqualValues(t, 2, statsIdemPGCountRows(t, db, ownerStatsTable),
		"同一 (epoch, interval, owner) 重复处理不能再多出行（旧实现这里是 4 行）")
	require.Zero(t, statsIdemPGDuplicateKeys(t, db, ownerStatsTable, "owner"))

	// 同一个 epoch 上的 24h 与 2880 是两个不同的 slot，互不干扰（interval 必须在自然键里）
	byDay := []*po.MinerStat{minerStatRow(2160, "2880", "f01000", 5)}
	require.NoError(t, d.SaveMinerStats(ctx, byDay))
	require.NoError(t, d.SaveMinerStats(ctx, byDay))
	require.EqualValues(t, 4, statsIdemPGCountRows(t, db, minerStatsTable), "24h 三行 + 2880 一行")
	require.Zero(t, statsIdemPGDuplicateKeys(t, db, minerStatsTable, "miner"))
}

// TestStatsIdemPGHealsLegacyDuplicatesAndKeepsOthers 生产现状（库里已有多行）：
// 重跑一次把该自然键收敛成一行，同时不碰同 slot 里没被重算的其他人。
func TestStatsIdemPGHealsLegacyDuplicatesAndKeepsOthers(t *testing.T) {
	db := statsIdemPGConnect(t)
	statsIdemPGSetup(t, db)
	d := NewMinerTaskDal(db)
	ctx := context.Background()

	for i := 0; i < 3; i++ { // epoch=2160 上同一个矿工被写了 3 遍（生产实测的形态）
		require.NoError(t, db.Exec(`insert into chain.miner_stats (epoch, miner, "interval", acc_reward) values (?, ?, ?, ?)`,
			2160, "f01000", "24h", i).Error)
	}
	require.NoError(t, db.Exec(`insert into chain.miner_stats (epoch, miner, "interval", acc_reward) values (?, ?, ?, ?)`,
		2160, "f01099", "24h", 42).Error)
	require.EqualValues(t, 4, statsIdemPGCountRows(t, db, minerStatsTable))
	require.EqualValues(t, 1, statsIdemPGDuplicateKeys(t, db, minerStatsTable, "miner"))

	// 只重算 f01000：它被收敛成一行，f01099 不动
	require.NoError(t, d.SaveMinerStats(ctx, []*po.MinerStat{minerStatRow(2160, "24h", "f01000", 9)}))

	require.EqualValues(t, 2, statsIdemPGCountRows(t, db, minerStatsTable), "重复行被收敛，同时不误删同 slot 的其他人")
	require.Zero(t, statsIdemPGDuplicateKeys(t, db, minerStatsTable, "miner"))

	var kept float64
	require.NoError(t, db.Raw(`select acc_reward from chain.miner_stats where epoch = 2160 and miner = 'f01099' and "interval" = '24h'`).Scan(&kept).Error)
	require.Equal(t, 42.0, kept, "没被传进来的矿工必须原样留着")
}

// TestStatsIdemPGWriterInsideCallerTransaction 调用方已经把事务塞在 ctx 里
// （_dal.ContextWithDB）时的行为：同一事务内重跑同一个 slot，提交后仍然只有一份行，
// 且「先删后插」的守卫没有因为嵌套事务而丢写。
func TestStatsIdemPGWriterInsideCallerTransaction(t *testing.T) {
	db := statsIdemPGConnect(t)
	statsIdemPGSetup(t, db)
	d := NewMinerTaskDal(db)

	batch := []*po.MinerStat{
		minerStatRow(2160, "24h", "f01000", 1),
		minerStatRow(2160, "24h", "f01001", 2),
	}

	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		ctx := _dal.ContextWithDB(context.Background(), tx)
		if err := d.SaveMinerStats(ctx, batch); err != nil {
			return err
		}
		if err := d.SaveOwnerStats(ctx, []*po.OwnerStat{ownerStatRow(2160, "2880", "f01000", 1)}); err != nil {
			return err
		}
		// 同一事务里再重跑一次同一个 slot
		return d.SaveMinerStats(ctx, batch)
	}))

	require.EqualValues(t, 2, statsIdemPGCountRows(t, db, minerStatsTable),
		"嵌套事务里重跑也必须只有一份行，且守卫不能把行删了就不插")
	require.EqualValues(t, 1, statsIdemPGCountRows(t, db, ownerStatsTable))
	require.Zero(t, statsIdemPGDuplicateKeys(t, db, minerStatsTable, "miner"))
}

// ---------------------------------------------------------------------------
// 交付给生产执行的那套 DDL（去重 + 逐分区 CREATE UNIQUE INDEX CONCURRENTLY + 父表元数据级
// 建索引）在真实 PG 上逐条验一遍。生产要执行的**就是**这份语句模板。
// ---------------------------------------------------------------------------

// statsIdemPGExecAutocommit 单条 DDL 直接下发（不开事务）：
// CREATE INDEX CONCURRENTLY 不允许在事务块里执行。
func statsIdemPGExecAutocommit(t *testing.T, db *gorm.DB, stmt string) error {
	t.Helper()
	return db.Session(&gorm.Session{SkipDefaultTransaction: true}).Exec(stmt).Error
}

// statsIdemPGPartitions 列出一张分区表的全部分区（与交付脚本里 pg_inherits 的查法一致）。
func statsIdemPGPartitions(t *testing.T, db *gorm.DB, parent string) []string {
	t.Helper()
	var names []string
	require.NoError(t, db.Raw(`
		select c.relname
		from pg_inherits i
		join pg_class c on c.oid = i.inhrelid
		where i.inhparent = ?::regclass
		order by c.relname`, parent).Scan(&names).Error)
	require.NotEmpty(t, names, "%s 应当有分区", parent)
	return names
}

// TestStatsIdemPGUniqueIndexRecipe 验证交付的 DDL 配方（在真实 PG 18 上逐条跑过）：
//
//  1. 分区父表上直接 `CREATE UNIQUE INDEX CONCURRENTLY` **会报错**
//     （ERROR: cannot create index on partitioned table ... concurrently），
//     所以必须逐分区建；
//  2. 去重 DML（DELETE ... USING 自连接 + ctid，按 epoch 范围收窄）能把历史重复行收敛成一行；
//  3. 逐分区 CONCURRENTLY + 父表一次普通 `CREATE UNIQUE INDEX`（元数据级、自动 attach）
//     ⇒ 父索引 valid、所有分区子索引都挂上；
//  4. 建好后重复插入会被 PG 直接拒绝（SQLSTATE 23505），这是写入守卫之外的兜底。
func TestStatsIdemPGUniqueIndexRecipe(t *testing.T) {
	db := statsIdemPGConnect(t)
	statsIdemPGSetup(t, db)

	type subject struct {
		table       string
		actorColumn string
		indexName   string
	}
	subjects := []subject{
		{minerStatsTable, "miner", "miner_stats_epoch_miner_interval_uindex"},
		{ownerStatsTable, "owner", "owner_stats_epoch_owner_interval_uindex"},
	}

	// 造历史重复行：每个自然键 3 行
	for _, s := range subjects {
		for i := 0; i < 3; i++ {
			for _, actor := range []string{"f01000", "f01001"} {
				require.NoError(t, db.Exec(fmt.Sprintf(
					`insert into %s (epoch, %s, "interval") values (?, ?, ?)`,
					s.table, s.actorColumn), 2160, actor, "24h").Error)
			}
		}
		require.EqualValues(t, 2, statsIdemPGDuplicateKeys(t, db, s.table, s.actorColumn),
			"%s：两个 actor 各被写了 3 遍 ⇒ 2 个自然键有重复", s.table)
	}

	// 1) 分区父表上直接 CONCURRENTLY 会失败 —— 这就是为什么要逐分区建
	for _, s := range subjects {
		err := statsIdemPGExecAutocommit(t, db,
			fmt.Sprintf(`create unique index concurrently %s on %s (epoch, %s, "interval")`, s.indexName, s.table, s.actorColumn))
		require.Error(t, err, "分区父表上 CREATE UNIQUE INDEX CONCURRENTLY 应当报错")
		require.Contains(t, err.Error(), "concurrently",
			"预期 PG 报 cannot create index on partitioned table ... concurrently，实际: %v", err)
	}

	// 2) 去重 DML（交付版本：按 epoch 范围收窄，避免全表自连接）
	for _, s := range subjects {
		require.NoError(t, db.Exec(fmt.Sprintf(`
			delete from %s a
			 using %s b
			 where a.epoch >= 0 and a.epoch < 2880
			   and a.epoch = b.epoch and a.%s = b.%s and a."interval" = b."interval"
			   and a.ctid < b.ctid`, s.table, s.table, s.actorColumn, s.actorColumn)).Error)
		require.Zero(t, statsIdemPGDuplicateKeys(t, db, s.table, s.actorColumn), "%s 去重后不应再有重复自然键", s.table)
		require.EqualValues(t, 2, statsIdemPGCountRows(t, db, s.table))
	}

	// 3) 逐分区 CONCURRENTLY + 父表元数据级建索引
	for _, s := range subjects {
		partitions := statsIdemPGPartitions(t, db, s.table)
		for _, p := range partitions {
			partIndex := fmt.Sprintf("%s_uindex", p)
			err := statsIdemPGExecAutocommit(t, db, fmt.Sprintf(
				`create unique index concurrently %s on chain.%s (epoch, %s, "interval")`, partIndex, p, s.actorColumn))
			require.NoError(t, err, "逐分区 CONCURRENTLY 应当成功: %s", partIndex)
		}

		require.NoError(t, statsIdemPGExecAutocommit(t, db, fmt.Sprintf(
			`create unique index %s on %s (epoch, %s, "interval")`, s.indexName, s.table, s.actorColumn)))

		var valid bool
		var children int64
		require.NoError(t, db.Raw(`
			select i.indisvalid, (select count(*) from pg_inherits where inhparent = i.indexrelid)
			from pg_index i where i.indexrelid = cast(? as regclass)`, "chain."+s.indexName).Row().Scan(&valid, &children))
		require.True(t, valid, "父索引 %s 必须 valid", s.indexName)
		require.EqualValues(t, len(partitions), children,
			"父索引 %s 必须把 %d 个分区子索引全挂上", s.indexName, len(partitions))
	}

	// 4) 兜底生效：重复插入被 PG 拒绝（epoch=2160 上 f01000 去重后还有一行）
	for _, s := range subjects {
		err := db.Exec(fmt.Sprintf(`insert into %s (epoch, %s, "interval") values (?, ?, ?)`,
			s.table, s.actorColumn), 2160, "f01000", "24h").Error
		require.Error(t, err, "唯一索引建好后重复插入必须报错")
		require.Contains(t, strings.ToLower(err.Error()), "duplicate key", "预期 23505 duplicate key，实际: %v", err)
	}

	// 5) 唯一索引在位时写入方依旧正常，且重复处理仍然只有一份
	d := NewMinerTaskDal(db)
	ctx := context.Background()
	require.NoError(t, d.SaveMinerStats(ctx, []*po.MinerStat{
		minerStatRow(4320, "24h", "f02000", 1),
		minerStatRow(4320, "24h", "f02001", 2),
	}))
	require.NoError(t, d.SaveMinerStats(ctx, []*po.MinerStat{
		minerStatRow(4320, "24h", "f02000", 1),
		minerStatRow(4320, "24h", "f02001", 2),
	}))
	require.EqualValues(t, 4, statsIdemPGCountRows(t, db, minerStatsTable), "2 行历史 + 2 行新写，重跑不新增")
	require.Zero(t, statsIdemPGDuplicateKeys(t, db, minerStatsTable, "miner"))
}
