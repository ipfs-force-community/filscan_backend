package rewardparity

import (
	"context"
	"fmt"
	"io"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gorm.io/gorm"
)

// Diagnose 打印「PG 侧一条都没读到」时最可能的三种原因对应的判据：
//
//  1. 覆盖缺口    —— 表内 epoch 根本没覆盖到区间（min/max 一眼可见）
//  2. 区间口径    —— 区间内总行数 > 0 但按区间读回 0 行 ⇒ 左右端点（[start,end) vs [start,end]）对不上
//  3. 地址形态    —— 点名矿工时按地址读 0 行，但区间内确实有该矿工的其它形态（前缀 / 大小写）
//
// 全部只读，且只查 chain.miner_rewards / chain.miner_win_counts 两张表。
type DiagnoseOptions struct {
	Miner       string // 带前缀或不带前缀都可（报告里两种形态都会打印）
	Start, End  int64
	DistinctMax int // 打印多少个区间内出现的 miner 值，<=0 取 20
}

func (o DiagnoseOptions) distinctMax() int {
	if o.DistinctMax <= 0 {
		return 20
	}
	return o.DistinctMax
}

// Diagnose 见类型注释。
func Diagnose(ctx context.Context, db *gorm.DB, o DiagnoseOptions, out io.Writer) {
	if db == nil || out == nil {
		return
	}
	fmt.Fprintln(out, "---- 诊断（只读） ----")

	for _, table := range []string{"chain.miner_rewards", "chain.miner_win_counts"} {
		var row struct {
			MinEpoch int64
			MaxEpoch int64
			Cnt      int64
		}
		if err := db.WithContext(ctx).Raw(dal.SQLTableRange(table)).Scan(&row).Error; err != nil {
			fmt.Fprintf(out, "%s 覆盖范围查询失败: %s\n", table, err)
			continue
		}
		fmt.Fprintf(out, "%s: epoch 覆盖 [%d, %d]，总行数 %d\n", table, row.MinEpoch, row.MaxEpoch, row.Cnt)
	}

	var rewardAll, rewardByMiner, winAll int64
	if err := db.WithContext(ctx).Raw(dal.SQLCountMinerRewardsRange, o.Start, o.End).Scan(&rewardAll).Error; err != nil {
		fmt.Fprintf(out, "chain.miner_rewards 区间行数查询失败: %s\n", err)
	}
	fmt.Fprintf(out, "chain.miner_rewards  区间 [%d,%d) 原始行数（全矿工）= %d\n", o.Start, o.End, rewardAll)

	if o.Miner != "" {
		norm := normalizeMiner(o.Miner)
		if err := db.WithContext(ctx).Raw(dal.SQLCountMinerRewardsRangeByMiner, norm, o.Start, o.End).Scan(&rewardByMiner).Error; err != nil {
			fmt.Fprintf(out, "chain.miner_rewards 按矿工行数查询失败: %s\n", err)
		}
		fmt.Fprintf(out, "chain.miner_rewards  区间 [%d,%d) 矿工 %s 的行数 = %d（0 且上面全矿工 >0 ⇒ 地址形态不匹配）\n",
			o.Start, o.End, norm, rewardByMiner)
	}

	if err := db.WithContext(ctx).Raw(dal.SQLCountMinerWinCountsRange, o.Start, o.End).Scan(&winAll).Error; err != nil {
		fmt.Fprintf(out, "chain.miner_win_counts 区间行数查询失败: %s\n", err)
	}
	fmt.Fprintf(out, "chain.miner_win_counts 区间 [%d,%d) 原始行数 = %d\n", o.Start, o.End, winAll)

	type dupRow struct {
		Epoch int64
		Miner string
		Cnt   int64
	}
	var dups []dupRow
	if err := db.WithContext(ctx).Raw(dal.SQLDuplicateMinerRewards, o.Start, o.End, 5).Scan(&dups).Error; err != nil {
		fmt.Fprintf(out, "chain.miner_rewards 重复行查询失败: %s\n", err)
	} else if len(dups) == 0 {
		fmt.Fprintln(out, "chain.miner_rewards 同 (epoch,miner) 重复行：无（唯一索引生效）")
	} else {
		fmt.Fprintf(out, "chain.miner_rewards 同 (epoch,miner) 重复行（最多 5 例，读路径已用 DISTINCT ON 兜住）：%v\n", dups)
	}

	dups = nil
	if err := db.WithContext(ctx).Raw(dal.SQLDuplicateMinerWinCounts, o.Start, o.End, 5).Scan(&dups).Error; err != nil {
		fmt.Fprintf(out, "chain.miner_win_counts 重复行查询失败: %s\n", err)
	} else if len(dups) == 0 {
		fmt.Fprintln(out, "chain.miner_win_counts 同 (epoch,miner) 重复行：无")
	} else {
		fmt.Fprintf(out, "chain.miner_win_counts 同 (epoch,miner) 重复行（最多 5 例，读路径已用 DISTINCT ON 兜住）：%v\n", dups)
	}

	var miners []string
	if err := db.WithContext(ctx).Raw(dal.SQLDistinctMinerInRewards, o.Start, o.End, o.distinctMax()).Scan(&miners).Error; err != nil {
		fmt.Fprintf(out, "chain.miner_rewards 区间内 miner 取值查询失败: %s\n", err)
	} else if len(miners) == 0 {
		fmt.Fprintln(out, "chain.miner_rewards 区间内 miner 取值：空")
	} else {
		fmt.Fprintf(out, "chain.miner_rewards 区间内 miner 取值样例（核对前缀形态，库内应为带前缀 f0…）: %v\n", miners)
	}
	fmt.Fprintln(out, "---- 诊断结束 ----")
}
