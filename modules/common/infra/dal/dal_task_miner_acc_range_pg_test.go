package dal

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// ---------------------------------------------------------------------------
// 真实 postgres 端到端验证（默认跳过，只在给出 DSN 时跑）
//
// 跑法（示例，**临时库**，跑完即 drop；绝不动生产表）：
//
//	createdb filscan_accrange_e2e
//	FILSCAN_ACC_RANGE_TEST_DSN='host=/tmp port=5432 user=postgres dbname=filscan_accrange_e2e sslmode=disable' \
//	    go test ./modules/common/infra/dal/ -run TestAccRangePGBoundary -count=1 -v
//	dropdb filscan_accrange_e2e
//
// 安全闸：本测试会 `drop schema chain cascade`，因此**只在库名含 accrange 的库上运行**，
// 连到别处直接 fail（防止误连生产/别的环境）。测试自身在 t.Cleanup 里也把新建的
// chain schema 删掉，环境不留痕。
// ---------------------------------------------------------------------------

const accPGDsnEnv = "FILSCAN_ACC_RANGE_TEST_DSN"

func accPGConnect(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv(accPGDsnEnv)
	if dsn == "" {
		t.Skipf("未设置 %s，跳过真实 postgres 端到端验证（跑法见本文件头注释）", accPGDsnEnv)
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: gormlogger.Discard})
	require.NoError(t, err)

	var name string
	require.NoError(t, db.Raw("select current_database()").Scan(&name).Error)
	require.Contains(t, strings.ToLower(name), "accrange",
		"安全闸：端到端测试会建/删 chain schema，只允许跑在库名含 accrange 的临时库上，本次是 %q", name)
	return db
}

// accPGSetup 在临时库里重建一个最小的 chain schema（三张表，列名与生产一致）
func accPGSetup(t *testing.T, db *gorm.DB) {
	t.Helper()
	stmts := []string{
		`drop schema if exists chain cascade`,
		`create schema chain`,
		`create table chain.miner_rewards (epoch bigint, miner varchar, reward numeric, block_count bigint)`,
		`create table chain.miner_win_counts (epoch bigint, miner varchar, win_count bigint)`,
		`create table chain.miner_gas_fees (epoch bigint, miner varchar, pre_agg numeric, prove_agg numeric, sector_gas numeric, wd_post_gas numeric, seal_gas numeric)`,
	}
	for _, s := range stmts {
		require.NoError(t, db.Exec(s).Error, s)
	}
	t.Cleanup(func() {
		require.NoError(t, db.Exec(`drop schema if exists chain cascade`).Error)
	})
}

// 真实 postgres 上的边界验证：窗口是 (2880, 4320]（左开右闭）
//
//   - epoch 2880：prevEpoch，属于**上一个**窗口 —— 旧实现会把它错算进来
//   - epoch 2881：本窗口第一格
//   - epoch 4320：本窗口最后一格（LteEnd）—— 旧实现会把它整格丢掉（可能整格都查不到）
//   - epoch 4321：属于下一个窗口
func TestAccRangePGBoundary(t *testing.T) {
	db := accPGConnect(t)
	accPGSetup(t, db)

	seed := []string{
		`insert into chain.miner_rewards (epoch, miner, reward, block_count) values
			(2880, 'f_prev',   1000, 10),
			(2880, 'f_shared',   100, 1),
			(2881, 'f_shared',    11, 1),
			(4320, 'f_shared',    22, 2),
			(4321, 'f_next',    3000, 30)`,
		`insert into chain.miner_win_counts (epoch, miner, win_count) values
			(2880, 'f_prev', 7), (2880, 'f_shared', 1), (2881, 'f_shared', 1), (4320, 'f_shared', 2), (4321, 'f_next', 3)`,
		`insert into chain.miner_gas_fees (epoch, miner, pre_agg, prove_agg, sector_gas, wd_post_gas, seal_gas) values
			(2880, 'f_prev', 1, 2, 3, 4, 5),
			(2880, 'f_shared', 10, 10, 10, 10, 10),
			(2881, 'f_shared', 20, 20, 20, 20, 20),
			(4320, 'f_shared', 30, 30, 30, 30, 30),
			(4321, 'f_next', 40, 40, 40, 40, 40)`,
	}
	for _, s := range seed {
		require.NoError(t, db.Exec(s).Error, s)
	}

	d := NewMinerTaskDal(db)
	rng := chain.NewLORCRange(2880, 4320)

	// 1) 累计奖励 / 出块：只有 f_shared 在窗口内有活动
	rewards, err := d.GetMinersAccRewards(context.Background(), rng)
	require.NoError(t, err)
	require.Len(t, rewards, 1, "窗口 (2880,4320] 内只有 f_shared 爆块（f_prev 在左端外、f_next 在右端外）")
	require.Equal(t, "f_shared", rewards[0].Miner)
	require.True(t, decimal.RequireFromString("33").Equal(rewards[0].Reward),
		"f_shared 的累计奖励应为 11+22=33，实际 %s（旧实现会是 111）", rewards[0].Reward)
	require.Equal(t, int64(3), rewards[0].BlockCount, "出块数应为 1+2=3（旧实现是 1+1=2）")

	// 2) 累计赢票
	wins, err := d.GetMinersAccWinCount(context.Background(), rng)
	require.NoError(t, err)
	require.Len(t, wins, 1)
	require.Equal(t, "f_shared", wins[0].Miner)
	require.Equal(t, int64(3), wins[0].WinCount, "赢票应为 1+2=3（旧实现是 1+1=2）")

	// 3) 累计 Gas
	fees, err := d.GetMinersAccGasFees(context.Background(), rng)
	require.NoError(t, err)
	require.Len(t, fees, 1)
	require.Equal(t, "f_shared", fees[0].Miner)
	require.True(t, decimal.RequireFromString("50").Equal(fees[0].SealGas),
		"seal_gas 应为 20+30=50，实际 %s（旧实现是 10+20=30）", fees[0].SealGas)
	require.True(t, decimal.RequireFromString("50").Equal(fees[0].WdPostGas),
		"wd_post_gas 应为 20+30=50，实际 %s", fees[0].WdPostGas)

	// 4) 同一条 SQL 直接对拍：把两种区间各数一遍，差恰好是「两端各一格」
	var prevRows, curRows int64
	require.NoError(t, db.Raw(`select count(*) from chain.miner_rewards where epoch >= 2880 and epoch < 4320`).Scan(&prevRows).Error)
	require.NoError(t, db.Raw(`select count(*) from chain.miner_rewards where epoch > 2880 and epoch <= 4320`).Scan(&curRows).Error)
	require.Equal(t, int64(3), prevRows, "旧区间 [2880,4320) 会多含左端 2880（f_prev/f_shared 两行）")
	require.Equal(t, int64(2), curRows, "正确区间 (2880,4320] 只含 2881 与 4320 两行")
}

// 真实 postgres 上按调用方的真实窗口（epoch-2880, epoch] 跑一遍：
// calc-miner-owner-task 在每个统计整点用 prevEpoch=epoch-2880 调用。
// 窗口两端各放一个**只有该高度才爆块的矿工**，用来钉死「哪一端属于本窗口」：
// 旧实现 [prev, epoch) 会把 f_left_only 算进来、把 f_right_only 丢掉 ⇒ 本用例必红。
func TestAccRangePGRealCallerWindow(t *testing.T) {
	db := accPGConnect(t)
	accPGSetup(t, db)

	const epoch = 4_700_000 // 统计整点
	const prev = epoch - 2880

	require.NoError(t, db.Exec(`insert into chain.miner_rewards (epoch, miner, reward, block_count) values (?, 'f_left_only', 1000, 1)`, prev).Error)
	require.NoError(t, db.Exec(`
		insert into chain.miner_rewards (epoch, miner, reward, block_count)
		select e, 'f_mid', 1, 1 from generate_series(?::bigint, ?::bigint) as e`, prev+1, epoch-1).Error)
	require.NoError(t, db.Exec(`insert into chain.miner_rewards (epoch, miner, reward, block_count) values (?, 'f_right_only', 1, 1)`, epoch).Error)

	rows, err := NewMinerTaskDal(db).GetMinersAccRewards(context.Background(), chain.NewLORCRange(chain.Epoch(prev), chain.Epoch(epoch)))
	require.NoError(t, err)

	got := map[string]int64{}
	var total int64
	for _, v := range rows {
		got[v.Miner] = v.BlockCount
		total += v.BlockCount
	}
	require.Equal(t, int64(2880), total, "窗口 (prev,epoch] 恰好 2880 格")
	require.Equal(t, int64(2879), got["f_mid"], "窗口内除两端外的 2879 格都是 f_mid")
	require.Equal(t, int64(1), got["f_right_only"], "右端 epoch（LteEnd）属于本窗口，必须算进来")
	require.NotContains(t, got, "f_left_only", "左端 prev 属于上一个窗口，不能算进 (prev,epoch]")

	// 端点归属：左端 prev 那一格不属于本窗口，右端 epoch 那一格属于本窗口
	var leftInWindow, rightInWindow bool
	require.NoError(t, db.Raw(`select exists(select 1 from chain.miner_rewards where epoch > ? and epoch <= ? and epoch = ?)`, prev, epoch, prev).Scan(&leftInWindow).Error)
	require.NoError(t, db.Raw(`select exists(select 1 from chain.miner_rewards where epoch > ? and epoch <= ? and epoch = ?)`, prev, epoch, epoch).Scan(&rightInWindow).Error)
	require.False(t, leftInWindow, "prevEpoch 属于上一个窗口")
	require.True(t, rightInWindow, "epoch（LteEnd）属于本窗口")
}
