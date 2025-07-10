package syncer

import (
	"context"
	"time"

	logging "github.com/gozelle/logger"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewTestContext(adapter londobell.Adapter, agg londobell.Agg, epoch chain.Epoch) *Context {
	return &Context{
		context:       context.Background(),
		adapter:       adapter,
		agg:           agg,
		epoch:         epoch,
		SugaredLogger: logging.NewLogger("simple-context").With(zap.String("epoch", epoch.String())),
	}
}

var _ logging.StandardLogger = (*Context)(nil)

type Context struct {
	context  context.Context
	db       *gorm.DB
	agg      londobell.Agg
	minerAgg londobell.MinerAgg
	adapter  londobell.Adapter
	epoch    chain.Epoch
	datamap  *Datamap
	start    time.Time
	*zap.SugaredLogger
	empty    bool
	epochs   chain.LCRCRange // 批量同步区间，左闭右闭
	lastCalc bool            // 是否为当批计算任务的最后一个
	dry      bool
}

func (c Context) MinerAgg() londobell.MinerAgg {
	return c.minerAgg
}

func (c Context) Dry() bool {
	return c.dry
}

// 表明本次并发同步的高度范围
func (c Context) Epochs() chain.LCRCRange {
	return c.epochs
}

// 表明是否是并发同步计算的最后一个高度
func (c Context) LastCalc() bool {
	return c.lastCalc
}

func (c Context) Empty() bool {
	return c.empty
}

func (c Context) SinceStart() time.Duration {
	return time.Since(c.start)
}

func (c Context) Datamap() *Datamap {
	return c.datamap
}

func (c Context) Context() context.Context {
	return c.context
}

func (c Context) Agg() londobell.Agg {
	return c.agg
}

func (c Context) Adapter() londobell.Adapter {
	return c.adapter
}

func (c Context) Epoch() chain.Epoch {
	return c.epoch
}
