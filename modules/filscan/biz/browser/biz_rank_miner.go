package browser

import (
	"context"
	"encoding/json"
	"github.com/dustin/go-humanize"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"
)

func NewMinerRankBiz(se repository.SyncEpochGetter, repo repository.MinerRankBizRepo, conf *config.Config) *MinerRankBiz {
	return &MinerRankBiz{se: se, repo: repo, redis: redis.NewRedis(conf)}
}

var _ filscan.MinerRankAPI = (*MinerRankBiz)(nil)

type MinerRankBiz struct {
	se    repository.SyncEpochGetter
	repo  repository.MinerRankBizRepo
	redis *redis.Redis
}

func (m MinerRankBiz) MinerRank(ctx context.Context, query filscan.PagingQuery) (resp *filscan.MinerRankResponse, err error) {
	
	err = query.Valid()
	if err != nil {
		return
	}
	
	epoch, err := m.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	
	if epoch == nil {
		return
	}
	
	var items []*bo.MinerRank
	var total int64
	items, total, err = m.repo.GetMinerRanks(ctx, *epoch, query)
	if err != nil {
		return
	}
	
	a := assembler.MinerRankAssembler{}
	resp, err = a.ToMinerRankResponse(total, query.Index, query.Limit, items)
	if err != nil {
		return
	}
	resp.UpdatedAt = epoch.Time().Unix()
	
	return
}

func (m MinerRankBiz) MinerPowerRank(ctx context.Context, query filscan.IntervalSectorPagingQuery) (resp *filscan.MinerPowerRankResponse, err error) {
	err = query.Valid()
	if err != nil {
		return
	}
	
	epoch, err := m.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	
	if epoch == nil {
		return
	}
	
	var it interval.Interval
	it, err = interval.ResolveInterval(query.Interval, *epoch)
	if err != nil {
		return
	}
	
	for {
		var t *chain.Epoch
		t, err = m.se.GetMinerEpochOrNil(ctx, it.Start())
		if err != nil {
			return
		}
		if t != nil {
			break
		}
		*epoch = *epoch - 120
		it, _ = interval.ResolveInterval(query.Interval, *epoch)
		if *epoch <= 0 {
			break
		}
	}
	
	if *epoch <= 0 {
		return
	}
	
	var sectorSize = uint64(0)
	if query.SectorSize != "" {
		sectorSize, err = humanize.ParseBytes(query.SectorSize)
		if err != nil {
			return
		}
	}
	
	var items []*bo.MinerPowerRank
	var total int64
	items, total, err = m.repo.GetMinerPowerRanks(ctx, *epoch, it.Start(), sectorSize, query.PagingQuery)
	if err != nil {
		return
	}
	
	a := assembler.MinerRankAssembler{}
	resp, err = a.ToMinerPowerRankResponse(total, query.Index, query.Limit, items)
	if err != nil {
		return
	}
	resp.UpdatedAt = epoch.Time().Unix()
	
	return
}

func (m MinerRankBiz) MinerRewardRank(ctx context.Context, query filscan.IntervalSectorPagingQuery) (resp *filscan.MinerRewardRankResponse, err error) {
	err = query.Valid()
	if err != nil {
		return
	}
	
	cacheKey, err := m.redis.HexCacheKey(ctx, query)
	if err != nil {
		return
	}
	cacheResult, err := m.redis.GetCacheResult(cacheKey)
	if err != nil {
		return
	}
	if cacheResult != nil {
		err = json.Unmarshal(cacheResult, &resp)
		if err != nil {
			return
		}
		return resp, nil
	}
	
	minerEpoch, err := m.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	
	if minerEpoch == nil {
		return
	}
	
	var sectorSize = uint64(0)
	if query.SectorSize != "" {
		sectorSize, err = humanize.ParseBytes(query.SectorSize)
		if err != nil {
			return
		}
	}
	
	var items []*bo.MinerRewardRank
	var total int64
	items, total, err = m.repo.GetMinerRewardRanks(ctx, query.Interval, *minerEpoch, sectorSize, query.PagingQuery)
	if err != nil {
		return
	}
	
	a := assembler.MinerRankAssembler{}
	resp, err = a.ToMinerRewardRankResponse(total, query.Index, query.Limit, items)
	if err != nil {
		return
	}
	resp.UpdatedAt = minerEpoch.Time().Unix()
	
	err = m.redis.Set(cacheKey, resp, chain.NextEpochInterval())
	if err != nil {
		return
	}
	
	return
}
