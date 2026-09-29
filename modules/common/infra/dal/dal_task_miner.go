package dal

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

func NewMinerTaskDal(db *gorm.DB) *MinerTaskDal {
	return &MinerTaskDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.MinerTask = (*MinerTaskDal)(nil)

type MinerTaskDal struct {
	*_dal.BaseDal
}

func (m MinerTaskDal) SaveAbsPower(ctx context.Context, powerIncrease, powerLoss decimal.Decimal, epoch int64) error {
	tx, err := m.DB(ctx)
	if err != nil {
		return err
	}
	return tx.Save(&po.AbsPowerChange{
		Epoch:         epoch,
		PowerIncrease: powerIncrease,
		PowerLoss:     powerLoss,
	}).Error
}

func (m MinerTaskDal) GetMinersAccWinCount(ctx context.Context, epochs chain.LORCRange) (items []*bo.AccWinCount, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
		select
		       miner,
		       sum(win_count) as win_count		
		from chain.miner_win_counts
		where epoch > ?
		  and epoch <= ?
		group by miner;
	`, epochs.GtBegin.Int64(), epochs.LteEnd.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerTaskDal) GetMinersAccGasFees(ctx context.Context, epochs chain.LORCRange) (fees []*bo.AccGasFee, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
		select
		       miner,
		       sum(pre_agg) as pre_agg,
		       sum(prove_agg) as prove_agg,
		       sum(sector_gas) as sector_gas,
		       sum(wd_post_gas) as wd_post_gas,
		       sum(seal_gas) as seal_gas
		from chain.miner_gas_fees
		where epoch > ?
		  and epoch <= ?
		group by miner;
	`, epochs.GtBegin.Int64(), epochs.LteEnd.Int64()).Find(&fees).Error
	if err != nil {
		return
	}

	return
}

func (m MinerTaskDal) GetMinersAccRewards(ctx context.Context, epochs chain.LORCRange) (rewards []*bo.AccReward, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
		select
		       miner,
		       sum(reward) as reward,
		       sum(block_count) as block_count
		from chain.miner_rewards
		where epoch > ?
		  and epoch <= ?
		group by miner;
	`, epochs.GtBegin.Int64(), epochs.LteEnd.Int64()).Find(&rewards).Error
	if err != nil {
		return
	}

	return
}

func (m MinerTaskDal) GetMinerInfosByEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.MinerInfo, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch = ?", epoch.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerTaskDal) GetOwnerInfosByEpoch(ctx context.Context, epoch chain.Epoch) (items []*po.OwnerInfo, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("epoch = ?", epoch.Int64()).Find(&items).Error
	if err != nil {
		return
	}
	return
}

func (m MinerTaskDal) SaveSyncMinerEpochPo(ctx context.Context, item *po.SyncMinerEpochPo) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.Create(item).Error
	})
	return
}

func (m MinerTaskDal) SaveOwnerInfos(ctx context.Context, infos []*po.OwnerInfo) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

func (m MinerTaskDal) SaveMinerInfos(ctx context.Context, infos []*po.MinerInfo) (err error) {
	err = m.Exec(ctx, func(tx *gorm.DB) error {
		return tx.CreateInBatches(infos, 100).Error
	})
	return
}

// SaveOwnerStats 幂等写入 chain.owner_stats。
//
// 背景（生产库实测）：chain.owner_stats 的自然键是 (epoch, interval, owner)，
// 但表上只有**非唯一**索引 owner_stats_epoch_owner_interval_index，同一自然键上
// 躺着多行（最老 slot epoch=2160 一行所有者被写了 3 次）；矿工/所有者累计页面是
// 把这些行**相加**的，于是每次重跑都会把重复行的值重复累加。原实现是裸
// CreateInBatches，同一 (epoch, interval) 重复处理一次就多一份行。
//
// 守卫（全部落在 DAL 层，调用方 syncer 一行不改）：同一事务内先按本批的自然键
// (epoch, interval, owner) **精确删掉**已存在的行，再批量插入；本批内部若自带
// 重复键，只保留最后一条。
//
// 为什么选「先删后插」而不是「先查后跳」：这两张表是可重算的派生统计快照，不是
// 事件流。重跑一段高度/一个 epoch 的真实场景正是「上一步算错了、要用新算法覆盖」
// （例如 59281ae 修的累计窗口左右端各偏一格）。「先查后跳」只是跳过已存在的行，
// 会把**旧的错值**继续留在库里、重跑不收敛；两种方案都不会再产生重复行，但只有
// 「先删后插」能让重跑收敛到最新计算值。删除严格按自然键匹配，所以传部分
// 所有者（分批/分页）时不会误删同 slot 里其他人的行。
//
// 并发与分区表：delete 与 insert 在同一个事务里，同一 (epoch, interval) 的并发
// 写入会先争同一批行锁，而不是各自插一份；delete 保留 `epoch = ?` 常量条件，分区
// 表只扫命中分区（裁剪）且 (epoch, owner, "interval") 上的既有索引可直接命中
// （PG18 实测 Index Scan）。真正的兜底是自然键唯一索引：交付说明里的
// CREATE UNIQUE INDEX 建好后，任何漏网的重复插入会直接报错，而不是继续静默累加。
func (m MinerTaskDal) SaveOwnerStats(ctx context.Context, stats []*po.OwnerStat) (err error) {
	rows := dedupeStatsByNaturalKey(stats, ownerStatNaturalKey)
	if len(rows) == 0 {
		// 空批（含全 nil）本就无行可写：直接返回，不再触发 gorm 的 empty slice 报错。
		return nil
	}

	db, err := m.DB(ctx)
	if err != nil {
		return err
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := deleteStatsByNaturalKey(tx, ownerStatsTable, ownerStatsActorColumn, ownerStatNaturalKeys(rows)); err != nil {
			return err
		}
		return statsIdemInsert(tx, rows, 100)
	})
	return
}

// SaveMinerStats 幂等写入 chain.miner_stats。语义、取舍与并发说明见 SaveOwnerStats
// （自然键是 (epoch, interval, miner)）。
func (m MinerTaskDal) SaveMinerStats(ctx context.Context, stats []*po.MinerStat) (err error) {
	rows := dedupeStatsByNaturalKey(stats, minerStatNaturalKey)
	if len(rows) == 0 {
		return nil
	}

	db, err := m.DB(ctx)
	if err != nil {
		return err
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := deleteStatsByNaturalKey(tx, minerStatsTable, minerStatsActorColumn, minerStatNaturalKeys(rows)); err != nil {
			return err
		}
		return statsIdemInsert(tx, rows, 100)
	})
	return
}

// ---------------------------------------------------------------------------
// 统计表写入的幂等守卫（只服务 SaveMinerStats / SaveOwnerStats）
// ---------------------------------------------------------------------------

const (
	minerStatsTable       = "chain.miner_stats"
	ownerStatsTable       = "chain.owner_stats"
	minerStatsActorColumn = "miner"
	ownerStatsActorColumn = "owner"

	// statsNaturalKeyChunk 单条 delete 里 actor in (...) 的个数上限，与
	// CreateInBatches 的批量大小 100 保持一致（PG 单语句参数上限 65535，远未触顶）。
	statsNaturalKeyChunk = 100
)

// statsIdemInsert 在守卫自己的事务里批量插入。
//
// 必须带上 SkipDefaultTransaction：gorm v1.24 的 CreateInBatches 内部会再开一层
// Transaction()，而在一层已经有事务、且「每语句自动开事务」还开着的时候，那层嵌套
// 会走 SAVEPOINT 分支（实测 DryRun 下只发出 SAVEPOINT 而**根本不发 INSERT**）。
// 关掉每语句自动事务后，分批 INSERT 直接落在守卫这层事务里：既保证 delete+insert
// 的原子性，又不会多出一层 savepoint。
func statsIdemInsert(tx *gorm.DB, rows interface{}, batchSize int) error {
	return tx.Session(&gorm.Session{SkipDefaultTransaction: true}).CreateInBatches(rows, batchSize).Error
}

// statsNaturalKey 统计表自然键 (epoch, interval, miner/owner)。interval 必须进键：
// epoch%2880==2160 时同一个 epoch 会分别写 interval='24h' 与 interval='2880' 两套行。
type statsNaturalKey struct {
	Epoch    int64
	Interval string
	Actor    string
}

func minerStatNaturalKey(s *po.MinerStat) (statsNaturalKey, bool) {
	if s == nil {
		return statsNaturalKey{}, false
	}
	return statsNaturalKey{Epoch: s.Epoch, Interval: s.Interval, Actor: s.Miner}, true
}

func ownerStatNaturalKey(s *po.OwnerStat) (statsNaturalKey, bool) {
	if s == nil {
		return statsNaturalKey{}, false
	}
	return statsNaturalKey{Epoch: s.Epoch, Interval: s.Interval, Actor: s.Owner}, true
}

func minerStatNaturalKeys(stats []*po.MinerStat) []statsNaturalKey {
	keys := make([]statsNaturalKey, 0, len(stats))
	for _, s := range stats {
		if k, ok := minerStatNaturalKey(s); ok {
			keys = append(keys, k)
		}
	}
	return keys
}

func ownerStatNaturalKeys(stats []*po.OwnerStat) []statsNaturalKey {
	keys := make([]statsNaturalKey, 0, len(stats))
	for _, s := range stats {
		if k, ok := ownerStatNaturalKey(s); ok {
			keys = append(keys, k)
		}
	}
	return keys
}

// dedupeStatsByNaturalKey 丢掉 nil 并按自然键去重：同一个键只保留**最后一次**
// 出现的那条（同一批里后算出来的版本更新），返回顺序保持稳定（按各键最后一次
// 出现的位置），便于断言与排查。
func dedupeStatsByNaturalKey[T any](items []T, keyOf func(T) (statsNaturalKey, bool)) []T {
	seen := make(map[statsNaturalKey]struct{}, len(items))
	rows := make([]T, 0, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		key, ok := keyOf(items[i])
		if !ok {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		rows = append(rows, items[i])
	}
	slices.Reverse(rows)
	return rows
}

// statsNaturalKeyGroup 一组共享同一 (epoch, interval) 的自然键。
type statsNaturalKeyGroup struct {
	Epoch    int64
	Interval string
	Actors   []string
}

// groupStatsNaturalKeys 按 (epoch, interval) 归组，组内保持输入顺序（分组顺序按
// 该组第一次出现的位置），这样一条 delete 只覆盖同 slot 的行、又不必为每个 actor
// 发一条语句。
func groupStatsNaturalKeys(keys []statsNaturalKey) []statsNaturalKeyGroup {
	type groupKey struct {
		Epoch    int64
		Interval string
	}

	indexOf := make(map[groupKey]int, len(keys))
	groups := make([]statsNaturalKeyGroup, 0, len(keys))
	for _, k := range keys {
		gk := groupKey{Epoch: k.Epoch, Interval: k.Interval}
		i, ok := indexOf[gk]
		if !ok {
			i = len(groups)
			indexOf[gk] = i
			groups = append(groups, statsNaturalKeyGroup{Epoch: k.Epoch, Interval: k.Interval})
		}
		groups[i].Actors = append(groups[i].Actors, k.Actor)
	}
	return groups
}

// deleteStatsByNaturalKey 把本批即将写入的自然键在库里**先删掉**（精确匹配，不按
// slot 整片删）：按 (epoch, interval) 归组、组内按 statsNaturalKeyChunk 分块下发。
func deleteStatsByNaturalKey(tx *gorm.DB, table, actorColumn string, keys []statsNaturalKey) error {
	for _, group := range groupStatsNaturalKeys(keys) {
		actors := group.Actors
		for len(actors) > 0 {
			n := statsNaturalKeyChunk
			if n > len(actors) {
				n = len(actors)
			}
			sql, args := statsNaturalKeyDeleteSQL(table, actorColumn, group.Epoch, group.Interval, actors[:n])
			if err := tx.Exec(sql, args...).Error; err != nil {
				return err
			}
			actors = actors[n:]
		}
	}
	return nil
}

// statsNaturalKeyDeleteSQL 生成一条「按自然键精确删除」的 SQL（纯函数，便于单测
// 直接钉住表名/列名/占位符）：
//
//	delete from <table> where epoch = ? and "interval" = ? and <actorColumn> in (?,...)
//
// epoch = ? 这个常量条件是刻意保留的：分区表按它裁剪（只扫命中分区），且
// (epoch, actor, "interval") 上的既有非唯一索引可以直接命中，不必全表/全分区扫。
func statsNaturalKeyDeleteSQL(table, actorColumn string, epoch int64, interval string, actors []string) (string, []interface{}) {
	sql := fmt.Sprintf(
		`delete from %s where epoch = ? and "interval" = ? and %s in (%s)`,
		table, actorColumn, strings.TrimSuffix(strings.Repeat("?,", len(actors)), ","),
	)

	args := make([]interface{}, 0, len(actors)+2)
	args = append(args, epoch, interval)
	for _, actor := range actors {
		args = append(args, actor)
	}
	return sql, args
}

func (m MinerTaskDal) DeleteMinerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_infos where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteOwnerInfos(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.owner_infos where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteSyncMinerEpochs(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.sync_miner_epochs where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteMinerStats(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_stats where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteOwnerStats(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.owner_stats where epoch >= ?`, gteEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteMinerStatsBeforeEpoch(ctx context.Context, ltEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.miner_stats where epoch < ? and interval != '2880'`, ltEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteOwnerStatsBeforeEpoch(ctx context.Context, ltEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.owner_stats where epoch < ? and interval != '2880'`, ltEpoch.Int64()).Error
	return
}

func (m MinerTaskDal) DeleteAbsPower(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Exec(`delete from chain.abs_power_change where epoch >= ?`, gteEpoch.Int64()).Error
	return
}
