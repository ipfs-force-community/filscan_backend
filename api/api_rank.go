package filscan

import (
	"context"
	"fmt"
	"github.com/shopspring/decimal"
)

type RankAPI interface {
	OwnerRankAPI
	MinerRankAPI
}

type OwnerRankAPI interface {
	OwnerRank(ctx context.Context, query PagingQuery) (resp *OwnerRankResponse, err error)
}

type MinerRankAPI interface {
	MinerRank(ctx context.Context, query PagingQuery) (resp *MinerRankResponse, err error)
	MinerPowerRank(ctx context.Context, query IntervalSectorPagingQuery) (resp *MinerPowerRankResponse, err error)
	MinerRewardRank(ctx context.Context, query IntervalSectorPagingQuery) (resp *MinerRewardRankResponse, err error)
}

type IntervalSectorPagingQuery struct {
	Interval   string `json:"interval"`
	SectorSize string `json:"sector_size"`
	PagingQuery
}

type PagingQuery struct {
	Index int          `json:"index"`
	Limit int          `json:"limit"`
	Order *PagingOrder `json:"order"`
}

func (p *PagingQuery) Valid() error {

	if p.Limit <= 0 {
		return fmt.Errorf("limit expect > 0, got: %d", p.Limit)
	}
	if p.Limit > 100 {
		return fmt.Errorf("limit expect <= 100, got: %d", p.Limit)
	}

	if p.Index < 0 {
		return fmt.Errorf("page expect >= 0, got: %d", p.Index)
	}

	return nil
}

type PagingOrder struct {
	Field string `json:"field"`
	Sort  string `json:"sort"`
}

// -----------------------排行榜页接口参数结构-----------------------

type MinerRankResponse struct {
	UpdatedAt int64        `json:"updated_at"`
	Total     int64        `json:"total"`
	Items     []*MinerRank `json:"items"` // 节点排行榜列表
}

type MinerPowerRankRequest struct {
	Interval   string `json:"interval"`    // 时间间隔：24h，7d，1m
	StartTime  string `json:"start_time"`  // 开始时间
	EndTime    string `json:"end_time"`    // 结束时间
	SectorSize int    `json:"sector_size"` // 扇区大小：32，64，默认为全部
}

type MinerPowerRankResponse struct {
	UpdatedAt int64             `json:"updated_at"`
	Total     int64             `json:"total"`
	Items     []*MinerPowerRank `json:"items"` // 节点增速排行榜列表
}

type MinerRewardRankRequest struct {
	Interval   string `json:"interval"`    // 时间间隔：24h，7d，1m
	StartTime  string `json:"start_time"`  // 开始时间
	EndTime    string `json:"end_time"`    // 结束时间
	SectorSize int    `json:"sector_size"` // 扇区大小：32，64，默认为全部
}

type MinerRewardRankResponse struct {
	UpdatedAt int64              `json:"updated_at"`
	Total     int64              `json:"total"`
	Items     []*MinerRewardRank `json:"items"` // 节点收益排行榜列表
}

// -----------------------排行榜页基础数据结构-----------------------

type OrePoolRank struct {
	OwnerID             string              `json:"owner_id"`              // 存储池号(OwnerID)
	QualityAdjPower     decimal.Decimal     `json:"quality_adj_power"`     // 有效算力
	MiningEfficiency24H decimal.NullDecimal `json:"mining_efficiency_24h"` // 近24小时产出效率
	PowerIncrease24H    decimal.NullDecimal `json:"power_increase_24h"`    // 近24小时增长算力
	BlockCount          int64               `json:"block_count"`           // 出块总数
}

type MinerRank struct {
	Rank              int             `json:"rank"`                // 排名
	MinerID           string          `json:"miner_id"`            // 节点号
	Balance           decimal.Decimal `json:"balance"`             // 余额
	QualityAdjPower   decimal.Decimal `json:"quality_adj_power"`   // 有效算力
	QualityPowerRatio decimal.Decimal `json:"quality_power_ratio"` // 有效算力占比
	PowerIncrease24H  decimal.Decimal `json:"power_increase_24h"`  // 近24小时增长算力
	BlockCount        int64           `json:"block_count"`         // 出块总数
	BlockRatio        decimal.Decimal `json:"block_ratio"`         // 出块总数占比
	Rewards           decimal.Decimal `json:"rewards"`             // 奖励总数
	RewardsRatio      decimal.Decimal `json:"rewards_ratio"`       // 奖励总数占比
}

type MinerPowerRank struct {
	Rank                 int             `json:"rank"`                   // 排名
	MinerID              string          `json:"miner_id"`               // 节点号
	PowerRatio           decimal.Decimal `json:"power_ratio"`            // 算力增速
	RawPower             decimal.Decimal `json:"raw_power"`              // 原值算力
	QualityAdjPower      decimal.Decimal `json:"quality_adj_power"`      // 有效算力
	QualityPowerIncrease decimal.Decimal `json:"quality_power_increase"` // 算力增量
	SectorSize           string          `json:"sector_size"`            // 扇区大小
}

type MinerRewardRank struct {
	Rank            int             `json:"rank"`
	MinerID         string          `json:"miner_id"`          // 节点号
	BlockCount      int64           `json:"block_count"`       // 出块总数
	Rewards         decimal.Decimal `json:"rewards"`           // 奖励总数
	RewardsRatio    decimal.Decimal `json:"rewards_ratio"`     // 奖励总数占比
	WinningRate     decimal.Decimal `json:"winning_rate"`      // 赢票率
	QualityAdjPower decimal.Decimal `json:"quality_adj_power"` // 有效算力
	SectorSize      string          `json:"sector_size"`       // 扇区大小
}

type OwnerRankResponse struct {
	UpdatedAt int64                    `json:"updated_at"`
	Total     int64                    `json:"total"` // 总数
	Items     []*OwnerRankResponseItem `json:"items"`
}

type OwnerRankResponseItem struct {
	Rank            int             `json:"rank"`              // 排名
	OwnerID         string          `json:"owner_id"`          // Owner ID
	QualityAdjPower decimal.Decimal `json:"quality_adj_power"` // 有效算力
	RewardsRatio24h decimal.Decimal `json:"rewards_ratio_24h"` // 近 24h 产出效率
	PowerChange24h  decimal.Decimal `json:"power_change_24h"`  // 近 24h 算力变化
	BlockCount      int64           `json:"block_count"`       // 出块数
}
