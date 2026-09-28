package metrics

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

// DefaultQueryTimeout 单条查询/接口调用的默认超时。
//
// 监控进程不得把业务库拖慢：任一条查询超时就放弃该条（计入 errors_total），
// 而不是无限等待。
const DefaultQueryTimeout = 15 * time.Second

// DBQuerier 生产实现：链头高度取自 londobell adapter，其余数据取自 filscan 业务库（PostgreSQL）。
//
// 关于为什么链头高度不走本仓同步器产物（这是旧口径坏掉的根因之一）：
// filscan_syncer_delay_height 类指标一旦用「同步器自己的进度」当基准（例如拿
// chain.sync_syncers 的两个不同 name 相减、或拿某个中间表与自己比），同步器停摆时
// 差值仍然是 0 附近的小数字，永远不会触发阈值。链头必须来自**独立数据源**
// ——londobell adapter 的 /adapter/epoch（它由抽取器/聚合器维护，与本仓同步器无关）。
type DBQuerier struct {
	db      *gorm.DB
	adapter londobell.Adapter
	timeout time.Duration
}

var _ Querier = (*DBQuerier)(nil)

// NewDBQuerier 新建查询器。adapter 允许为 nil（此时链头高度不可用，采集器会以
// filscan_metrics_up=0 + errors_total{scope="head"} 如实暴露，而不是伪造一个高度）。
func NewDBQuerier(db *gorm.DB, adapter londobell.Adapter, timeout time.Duration) *DBQuerier {
	if timeout <= 0 {
		timeout = DefaultQueryTimeout
	}
	return &DBQuerier{db: db, adapter: adapter, timeout: timeout}
}

// HeadHeight 链头高度：londobell adapter GET /adapter/epoch（epoch 传 null 取最新）。
func (q *DBQuerier) HeadHeight(ctx context.Context) (int64, time.Time, error) {
	if q.adapter == nil {
		return 0, time.Time{}, errors.New("londobell adapter 未配置（配置缺少 [londobell].adapter_address）")
	}
	ctx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()
	reply, err := q.adapter.Epoch(ctx, nil)
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("取链头高度失败: %w", err)
	}
	if reply == nil {
		return 0, time.Time{}, errors.New("取链头高度失败: 适配器返回空响应")
	}
	return reply.Epoch, reply.BlockTime, nil
}

// Syncers 读取同步器游标：chain.sync_syncers(name, epoch)。
func (q *DBQuerier) Syncers(ctx context.Context) ([]SyncerCursor, error) {
	db, err := q.sqlDB()
	if err != nil {
		return nil, err
	}
	table, err := QuoteQualified(po.SyncSyncer{}.TableName())
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()

	rows, err := db.QueryContext(ctx, fmt.Sprintf(`select "name", "epoch" from %s`, table))
	if err != nil {
		return nil, fmt.Errorf("查询同步器游标失败: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []SyncerCursor
	for rows.Next() {
		var item SyncerCursor
		if err := rows.Scan(&item.Name, &item.Epoch); err != nil {
			return nil, fmt.Errorf("读取同步器游标失败: %w", err)
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历同步器游标失败: %w", err)
	}
	return out, nil
}

// MaxHeight 读取某表高度列的最大值。表不存在/超时等错误原样返回（由采集器分类并容错）。
func (q *DBQuerier) MaxHeight(ctx context.Context, spec TableSpec) (int64, bool, error) {
	if err := spec.Validate(); err != nil {
		return 0, false, err
	}
	db, err := q.sqlDB()
	if err != nil {
		return 0, false, err
	}
	ctx, cancel := context.WithTimeout(ctx, q.timeout)
	defer cancel()

	var v sql.NullInt64
	if err := db.QueryRowContext(ctx, spec.MaxSQL()).Scan(&v); err != nil {
		return 0, false, fmt.Errorf("查询 %s 新鲜度失败: %w", spec.String(), err)
	}
	if !v.Valid {
		return 0, false, nil
	}
	return v.Int64, true, nil
}

func (q *DBQuerier) sqlDB() (*sql.DB, error) {
	if q.db == nil {
		return nil, errors.New("数据库未初始化")
	}
	return q.db.DB()
}
