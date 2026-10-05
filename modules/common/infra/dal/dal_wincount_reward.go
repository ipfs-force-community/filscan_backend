package dal

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

// 本文件：首页「每赢票奖励」实测口径（Δ矿工实收 ÷ Δ赢票数）的 PG 读实现（只读）。
//
// 口径依据（与 dal_biz_miner_reward_range.go 的 SQLMinerWinCountsRange 完全一致）：
// chain.miner_win_counts 建的是**非唯一**索引、写路径是纯 INSERT，同一高度重跑同步会在同一
// (epoch, miner) 上留下多行**完全相同**的 win_count。若直接 sum(win_count)，赢票数会被放大 N 倍
// （N=重跑次数），把「每赢票奖励」算小。故必须先 DISTINCT ON (epoch, miner) 收敛成「每个
// (epoch,miner) 一行」，再求和。
//
// covered_epochs 用 count(distinct epoch)：该表每个高度可能有多个矿工各一行，
// 覆盖率关心的是「窗口里有多少个高度真的有数据」，不是行数。

// SQLWinCountRewardStats 区间 [start, end) 内去重后的赢票总数 + 覆盖高度数。
const SQLWinCountRewardStats = `
select coalesce(sum(win_count), 0) as sum_win_count,
       count(distinct epoch)      as covered_epochs
from (select distinct on (epoch, miner)
             epoch,
             miner,
             win_count
      from chain.miner_win_counts
      where epoch >= ?
        and epoch < ?
      order by epoch, miner) t`

func NewWinCountRewardDal(db *gorm.DB) *WinCountRewardDal {
	return &WinCountRewardDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.WinCountReward = (*WinCountRewardDal)(nil)

// WinCountRewardDal 首页「每赢票奖励」赢票数的 PG 读实现。
type WinCountRewardDal struct {
	*_dal.BaseDal
}

func (w WinCountRewardDal) GetWinCountRewardStats(ctx context.Context, start, end chain.Epoch) (sumWinCount int64, coveredEpochs int64, err error) {
	tx, err := w.DB(ctx)
	if err != nil {
		return
	}
	var row struct {
		SumWinCount   int64 `gorm:"column:sum_win_count"`
		CoveredEpochs int64 `gorm:"column:covered_epochs"`
	}
	err = tx.Raw(SQLWinCountRewardStats, start.Int64(), end.Int64()).Scan(&row).Error
	if err != nil {
		return
	}
	sumWinCount = row.SumWinCount
	coveredEpochs = row.CoveredEpochs
	return
}
