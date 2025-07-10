package prorepo

import (
	"context"

	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type MinerRepo interface {
	GetProInfoEpoch(ctx context.Context) (epoch chain.Epoch, err error)
	GetMinerInfos(ctx context.Context, epoch int64, miners []string) (infos map[string]*propo.MinerInfo, err error)
	GetMinerBalances(ctx context.Context, epoch int64, miners []string) (balances map[string]*propo.MinerBalance, err error)
	// TODO check 此处采用左开右闭规则查询，因为此处数据库采用了最后一个高度表示前 1 个小时所有的高度的 Fee 之和
	GetMinerFunds(ctx context.Context, epochs chain.LORCRange, miners []string) (fees map[string]*propo.MinerFund, err error)
	GetMinerAccReward(ctx context.Context, miners string) (r map[string]decimal.Decimal, err error)
	GetMinersSectors(ctx context.Context, epoch int64, miners []string) (sectors []*propo.MinerSector, err error)
	GetSyncEpoch(ctx context.Context) (epoch int64, err error)
}

type LuckRepo interface {
	QueryMinerLucks(ctx context.Context, miners []string, epoch int64) (m map[string]*Luck, err error)
}

type Luck struct {
	Miner   string
	Luck24h decimal.Decimal
	Luck7d  decimal.Decimal
	Luck30d decimal.Decimal
}
