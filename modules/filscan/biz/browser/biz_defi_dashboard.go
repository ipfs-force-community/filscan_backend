package browser

import (
	"context"
	"fmt"
	"sort"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/defi_task/themis_pro"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type DefiDashboardBiz struct {
	agg         londobell.Agg
	adapter     londobell.Adapter
	repo        repository.DefiRepo
	evmTransfer *dal.EVMTransferDal
}

func (d DefiDashboardBiz) DefiSummary(ctx context.Context, _ struct{}) (*filscan.DefiSummaryReply, error) {
	max, err := d.repo.GetMaxHeight(ctx)
	if err != nil {
		return nil, err
	}

	dbRes, err := d.repo.GetAllItemsOnEpoch(ctx, max)
	if err != nil {
		return nil, err
	}
	dbRes23hAgo, err := d.repo.GetAllItemsOnEpoch(ctx, max-2880)
	if err != nil {
		return nil, err
	}
	FevmStaked, FevmStaked24hAgo, filStaked := decimal.Zero, decimal.Zero, decimal.Zero
	user, user24hAgo := 0, 0
	for i := range dbRes {
		FevmStaked = FevmStaked.Add(dbRes[i].Tvl)
		filStaked = filStaked.Add(dbRes[i].TvlInFil)
		user += dbRes[i].Users
	}

	for i := range dbRes23hAgo {
		FevmStaked24hAgo = FevmStaked24hAgo.Add(dbRes23hAgo[i].Tvl)
		user24hAgo += dbRes23hAgo[i].Users
	}

	return &filscan.DefiSummaryReply{
		FevmStaked:        FevmStaked,
		StakedChangeIn24h: FevmStaked.Sub(FevmStaked24hAgo),
		TotalUser:         user,
		UserChangeIn24h:   user - user24hAgo,
		FilStaked:         filStaked,
		UpdatedAt:         chain.Epoch(max).Unix(),
	}, nil
}

func (d DefiDashboardBiz) DefiProtocolList(ctx context.Context, req filscan.DefiDashboardListRequest) (*filscan.DefiDashboardListResponse, error) {
	filed := req.Field
	if filed != "tvl" && filed != "tvl_change_rate_in_24h" && filed != "tvl_change_in_24h" && filed != "users" {
		return nil, fmt.Errorf("invalid filed value")
	}

	max, err := d.repo.GetMaxHeight(ctx)
	if err != nil {
		return nil, err
	}

	dbRes, err := d.repo.GetAllItemsOnEpoch(ctx, max)
	if err != nil {
		return nil, err
	}
	dbRes23hAgo, err := d.repo.GetAllItemsOnEpoch(ctx, max-2880)
	if err != nil {
		return nil, err
	}
	tmp := []filscan.DefiItems{}

	mp := map[string]*po.DefiDashboard{}
	for i := range dbRes23hAgo {
		mp[dbRes23hAgo[i].Protocol] = dbRes23hAgo[i]
	}
	for i := range dbRes {
		var dd *po.DefiDashboard
		var ok bool
		if dd, ok = mp[dbRes[i].Protocol]; !ok {
			dd = &po.DefiDashboard{
				Tvl: decimal.Zero,
			}
		}
		tvlChange := dbRes[i].Tvl.Sub(dd.Tvl)
		var tvlChangeRate decimal.Decimal
		if !dbRes[i].Tvl.Equal(decimal.Zero) {
			tvlChangeRate = tvlChange.Div(dbRes[i].Tvl).Mul(decimal.NewFromInt(100)).Round(2)
		} else {
			tvlChangeRate = decimal.Zero
			log.Warn("db res tvl meet zero")
		}
		st := filscan.StakedToken{
			TokenName: "FIL",
			IconUrl:   "https://filscan-v2.oss-cn-hongkong.aliyuncs.com/fvm_manage/images/filecoin.svg",
			Rate:      100,
		}
		themis := themis_pro.ThemisPro{}
		if dbRes[i].Protocol == themis.GetProtocolName() {
			st = filscan.StakedToken{
				TokenName: "THS",
				IconUrl:   "https://filscan-v2.oss-cn-hongkong.aliyuncs.com/fvm_manage/images/THS.png",
				Rate:      100,
			}
		}

		//Special handling of FILLiquid according to customer requirements
		if dbRes[i] != nil && dbRes[i].Protocol != "" && dbRes[i].Protocol == "FILLiquid" {
			evmTransferStats, err := d.evmTransfer.GetEvmTransferStatsByContractName(ctx, dbRes[i].Protocol)
			if err != nil {
				return nil, err
			}
			dbRes[i].Users = int(evmTransferStats.AccUserCount)
			fmt.Printf("FILLiquid account is: %v\n", evmTransferStats.AccUserCount)
		}

		tmp = append(tmp, filscan.DefiItems{
			Protocol:           dbRes[i].Protocol,
			Tvl:                dbRes[i].Tvl,
			TvlChangeRateIn24h: tvlChangeRate,
			TvlChangeIn24h:     tvlChange,
			Users:              dbRes[i].Users,
			IconUrl:            dbRes[i].Url,
			Tokens: []filscan.StakedToken{
				st,
			},
			MainSite: d.repo.GetProductMainSite(ctx, dbRes[i].Protocol),
		})
	}

	reverse := req.Reverse
	sort.Slice(tmp, func(i, j int) bool {

		if filed == "tvl" {
			return tmp[i].Tvl.LessThan(tmp[j].Tvl) == reverse
		}

		if filed == "tvl_change_rate_in_24h" || filed == "tvl_change_in_24h" {
			return tmp[i].TvlChangeIn24h.LessThan(tmp[j].TvlChangeIn24h) == reverse
		}

		return tmp[i].Users < tmp[j].Users == reverse
	})

	start := req.Page * req.Limit
	end := (req.Page + 1) * req.Limit
	if end > len(tmp) {
		end = len(tmp)
	}
	return &filscan.DefiDashboardListResponse{
		Total: len(dbRes),
		Items: tmp[start:end],
	}, nil
}

func NewDefiDashboardBiz(agg londobell.Agg, adapter londobell.Adapter, db *gorm.DB) *DefiDashboardBiz {
	return &DefiDashboardBiz{
		agg:         agg,
		adapter:     adapter,
		repo:        dal.NewDefiDashboardDal(db, dal.NewERC20Dal(db)),
		evmTransfer: dal.NewEVMTransferDal(db),
	}
}

var _ filscan.DefiDashboardAPI = (*DefiDashboardBiz)(nil)
