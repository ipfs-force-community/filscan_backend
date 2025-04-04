package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

type ResourceBiz struct {
	db         *gorm.DB
	repo       repository.ResourceRepo
	eventsRepo repository.EventsRepo
	defiRepo   repository.DefiRepo
	syncEpoch  repository.SyncerGetter
	adapter    *acl.StatisticAclImpl
	agg        londobell.Agg
	filPrice   repository.FilPriceRepo
}

var VestReleaseDateResp = []filscan.ReleaseItem{}
var VestReleaseDateRespLock sync.RWMutex

func (r ResourceBiz) VestReleaseDate(ctx context.Context, _ struct{}) (*filscan.ReleaseDate, error) {
	VestReleaseDateRespLock.RLock()
	defer VestReleaseDateRespLock.RUnlock()
	res := VestReleaseDateResp
	return &filscan.ReleaseDate{ReleaseItemList: res}, nil
}

func (r ResourceBiz) FilecoinBaseData(ctx context.Context, _ struct{}) (*filscan.FilecoinBaseDataReply, error) {
	fp, err := r.adapter.GetFilCompose(ctx)
	if err != nil {
		return nil, err
	}
	p, err := r.filPrice.LatestPrice(ctx)
	if err != nil {
		return nil, err
	}
	tokenPrice := decimal.NewFromFloat(p.Price)

	fil := decimal.New(1, 18)
	resp, err := http.Get("https://dncapi.shermanantitrustact.com/api/coin/cointrades-web?code=filecoinnew&webp=1")
	if err != nil {
		log.Errorf("get info from dncapi failed: %w", err)
		return nil, err
	}
	bs, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("io read all: %w", err)
		return nil, err
	}

	result := struct {
		Data []struct {
			Name       string  `json:"name"`
			Percent    float64 `json:"percent"`
			Volume     float64 `json:"volume"`
			VolumeBtc  float64 `json:"volume_btc"`
			Amount     float64 `json:"amount"`
			NativeName string  `json:"native_name"`
		} `json:"data"`
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}{}

	err = json.Unmarshal(bs, &result)
	if err != nil {
		return nil, err
	}

	sum := 0.0
	for i := range result.Data {
		sum += result.Data[i].Volume
	}
	rmbPrice := struct {
		Data struct {
			Usdt float64 `json:"usdt_price_cny"`
		} `json:"data"`
	}{}

	resp, err = http.Get("https://dncapi.shermanantitrustact.com/api/home/global?webp=1")
	if err != nil {
		log.Errorf("get info from dncapi failed: %w", err)
		return nil, err
	}
	bs, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("io read all: %w", err)
		return nil, err
	}

	err = json.Unmarshal(bs, &rmbPrice)
	if err != nil {
		return nil, err
	}
	resp, err = http.Get("https://dncapi.shermanantitrustact.com/api/coin/coinhisrank?code=filecoinnew&webp=1")

	if err != nil {
		log.Errorf("get info from dncapi failed: %w", err)
		return nil, err
	}
	bs, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Errorf("io read all: %w", err)
		return nil, err
	}

	autoGenerated := struct {
		Data []struct {
			Code       string `json:"code"`
			TickerTime string `json:"ticker_time"`
			RankNo     int    `json:"rank_no"`
		} `json:"data"`
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}{}
	err = json.Unmarshal(bs, &autoGenerated)
	if err != nil {
		log.Errorf("json marshal failed: %w", err)
		return nil, err
	}
	return &filscan.FilecoinBaseDataReply{
		Locked:            fp.Locked.Add(fp.RemainingVested).Add(fp.RemainingReserved).Div(fil).InexactFloat64(),
		ChangedAmount:     sum / p.Price,
		Vol:               (2000000000 - fp.Burnt.Div(fil).InexactFloat64()) * p.Price,
		TxsRate:           0,
		CirculatingRate:   fp.Circulating.Div(fil).Div(decimal.NewFromInt(2000000000)).InexactFloat64() * 100,
		Burn:              fp.Burnt.Div(fil).InexactFloat64(),
		LockedRate:        fp.Locked.Add(fp.RemainingVested).Add(fp.RemainingReserved).Div(fil).Div(decimal.NewFromInt(2000000000)).InexactFloat64() * 100,
		MaxSupply:         2000000000,
		BurnRate:          fp.Burnt.Div(fil).Div(decimal.NewFromInt(2000000000)).InexactFloat64() * 100,
		Circulating:       fp.Circulating.Div(fil).Mul(tokenPrice).InexactFloat64(),
		ChangedVol:        sum,
		ChangeRate:        sum / fp.Circulating.Div(fil).Mul(tokenPrice).InexactFloat64(),
		CirculatingAmount: fp.Circulating.Div(fil).InexactFloat64(),
		Price:             p.Price,
		PriceChangeRate:   p.PercentChange,
		RmbPrice:          p.Price * rmbPrice.Data.Usdt,
		Rank:              autoGenerated.Data[len(autoGenerated.Data)-1].RankNo + 1,
	}, nil
}

func (r ResourceBiz) TokenHolderTrend(ctx context.Context, req *filscan.TokenHolderTrendReq) (*[]filscan.TokenHolderTrendRes, error) {
	resp, err := http.Get("https://dncapi.shermanantitrustact.com/api/v3/coin/holders?code=filecoinnew&webp=1")
	if err != nil {
		log.Errorf("request from dncapi failed", err)
		return nil, err
	}
	type List struct {
		Updatedate int     `json:"updatedate"`
		Addrcount  int     `json:"addrcount"`
		Top10Rate  float64 `json:"top10rate"`
		Top20Rate  float64 `json:"top20rate"`
		Top50Rate  float64 `json:"top50rate"`
		Top100Rate float64 `json:"top100rate"`
	}
	type Holdcoin struct {
		List []List `json:"list"`
	}
	type Data struct {
		Holdcoin Holdcoin `json:"holdcoin"`
	}
	type AutoGenerated struct {
		Data Data   `json:"data"`
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}

	resps := AutoGenerated{}
	rbs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = json.Unmarshal(rbs, &resps)
	if err != nil {
		log.Errorf("marshal into resps failed: %w", err)
		return nil, err
	}
	reply := []filscan.TokenHolderTrendRes{}
	for _, v := range resps.Data.Holdcoin.List {
		reply = append(reply, filscan.TokenHolderTrendRes{
			Timpstamp:  v.Updatedate,
			Addrcount:  v.Addrcount,
			Top10Rate:  v.Top10Rate,
			Top20Rate:  v.Top20Rate,
			Top50Rate:  v.Top50Rate,
			Top100Rate: v.Top100Rate,
		})
	}
	return &reply, nil
}

func (r ResourceBiz) TopActiveAddress(ctx context.Context, _ struct{}) (*[]filscan.Flows, error) {
	resp, err := http.Get("https://dncapi.shermanantitrustact.com/api/v3/coin/holders?code=filecoinnew&webp=1")
	if err != nil {
		log.Errorf("request from dncapi failed", err)
		return nil, err
	}

	resps := struct {
		Data struct {
			Flows []filscan.Flows `json:"toplist"`
		} `json:"data"`
	}{}

	rbs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = json.Unmarshal(rbs, &resps)
	if err != nil {
		log.Errorf("marshal into resps failed: %w", err)
		return nil, err
	}

	resps.Data.Flows = resps.Data.Flows[:10]
	return &resps.Data.Flows, nil
}

func (r ResourceBiz) TokenHolderAddress(ctx context.Context, _ struct{}) (*filscan.TokenHolderAddressRes, error) {
	resp, err := http.Get("https://dncapi.shermanantitrustact.com/api/v3/coin/holders?code=filecoinnew&webp=1")
	if err != nil {
		log.Errorf("request from dncapi failed", err)
		return nil, err
	}

	resps := struct {
		Data struct {
			Top filscan.TokenHolderAddressRes `json:"top"`
		} `json:"data"`
	}{}
	rbs, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	err = json.Unmarshal(rbs, &resps)
	if err != nil {
		log.Errorf("marshal into resps failed: %w", err)
		return nil, err
	}

	return &resps.Data.Top, nil
}

func (r ResourceBiz) GetFilecoinTrend(ctx context.Context, req *filscan.GetFilecoinTrendReq) (*filscan.GetFilecoinTrendRes, error) {
	resp, err := http.Get(fmt.Sprintf("https://dncapi.bostonteapartyevent.com/api/coin/web-charts?code=%s&type=%s&webp=%d",
		req.Code, req.Type, req.Webp))
	if err != nil {
		return nil, err
	}

	rb, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	return &filscan.GetFilecoinTrendRes{Data: string(rb)}, nil
}

func (r ResourceBiz) GetFilecoinChange(ctx context.Context, req *filscan.GetFilecoinChangeReq) (*filscan.GetFilecoinChangeRes, error) {
	resp, err := http.Get(fmt.Sprintf("https://dncapi.shermanantitrustact.com/api/coin/coinchange?code=%s&webp=%d",
		req.Code, req.Webp))
	if err != nil {
		return nil, err
	}

	rb, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	return &filscan.GetFilecoinChangeRes{Data: string(rb)}, nil
}

func (r ResourceBiz) GetFilecoinKLine(ctx context.Context, req *filscan.GetFilecoinKLineReq) (*filscan.GetFilecoinKLineRes, error) {
	resp, err := http.Get(fmt.Sprintf("https://dncapi.shermanantitrustact.com/api/v1/kline/market?tickerid=%s&period=%d&reach=%d&since=%s&utc=%d&webp=%d",
		req.TikckerId, req.Period, req.Reach, req.Since, req.Utc, req.Webp))
	if err != nil {
		return nil, err
	}

	rb, err := io.ReadAll(resp.Body)
	defer resp.Body.Close()
	if err != nil {
		return nil, err
	}

	return &filscan.GetFilecoinKLineRes{Data: string(rb)}, nil
}

func (r ResourceBiz) NetworkCapital(ctx context.Context, _ struct{}) (*filscan.NetworkCapitalReply, error) {
	var tipset []*londobell.Tipset
	latestTipset, err := r.agg.LatestTipset(ctx)
	if err != nil {
		return nil, err
	}
	if latestTipset == nil {
		err = fmt.Errorf("latest tipset is empty")
		return nil, err
	} else {
		tipset = latestTipset
	}
	epoch := chain.Epoch(tipset[0].ID)
	preEpoch := epoch - 2880
	compose, err := r.adapter.GetFilComposeByEpoch(ctx, &epoch)
	if err != nil {
		return nil, err
	}
	preCompose, err := r.adapter.GetFilComposeByEpoch(ctx, &preEpoch)
	if err != nil {
		return nil, err
	}
	tvl, tvl_24h, err := r.defiRepo.GetMaxHeight24hTvl(ctx) //一小时数据才同步一次，因此不直接使用上面的epoch
	if err != nil {
		return nil, err
	}
	tvl = tvl.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	tvl_24h = tvl_24h.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	vested24h := compose.Vested.Sub(preCompose.Vested)
	mined24h := compose.Mined.Sub(preCompose.Mined)
	reserved24h := compose.ReserveDisbursed.Sub(preCompose.ReserveDisbursed)
	pledge24h := compose.Locked.Sub(preCompose.Locked)
	burn24h := compose.Burnt.Sub(preCompose.Burnt)
	circulation24h := compose.Circulating.Sub(preCompose.Circulating)
	res := &filscan.NetworkCapitalReply{
		FilProduce:     compose.Mined.Add(compose.Vested).Add(compose.ReserveDisbursed),
		FilProduce24h:  mined24h.Add(vested24h).Add(reserved24h),
		Mined:          compose.Mined,
		Mined24h:       mined24h,
		Vested:         compose.Vested,
		Vested24h:      vested24h,
		Reserved:       compose.ReserveDisbursed,
		Reserved24h:    reserved24h,
		Locked:         tvl.Add(compose.Locked),
		Locked24h:      tvl_24h.Add(pledge24h),
		Pledge:         compose.Locked,
		Pledge24h:      pledge24h,
		DefiTvl:        tvl,
		DefiTvl24h:     tvl_24h,
		Burn:           compose.Burnt,
		Burn24h:        burn24h,
		Circulation:    compose.Circulating,
		Circulation24h: circulation24h,
	}
	return res, nil
}

func (r ResourceBiz) NetworkCapitalFigure(ctx context.Context, req *filscan.NetworkCapitalFigureReq) (resp *filscan.NetworkCapitalFigureReply, err error) {
	var epoch int64
	err = r.db.Raw("select max(epoch) as ma from fevm.defi_dashboard").Scan(&epoch).Error
	if err != nil {
		return
	}

	var intervalPointsAGG interval.Interval
	//todo 数据同步时间不一致。。。。。。。。。
	diff := int64(120 - (epoch % 120))
	intervalPointsAGG, err = interval.ResolveInterval(req.Interval, chain.Epoch(epoch+diff).CurrentHour())
	if err != nil {
		return
	}
	var epochsDB []chain.Epoch
	epochsAGG := intervalPointsAGG.Points()

	epochsDB = make([]chain.Epoch, len(epochsAGG))
	for i, epoch := range epochsAGG {
		epochsDB[i] = epoch - chain.Epoch(diff)
	}
	tvls, err := r.defiRepo.GetTvlByEpochs(ctx, epochsDB)
	if err != nil {
		return
	}
	filsupplys, err := r.agg.GetFilsupply(ctx, epochsAGG)
	if err != nil {
		return
	}
	//if len(tvls) != len(filsupplys) {
	//	return nil, fmt.Errorf("get res error: the two are inconsistent")
	//}
	sort.Slice(tvls, func(i, j int) bool {
		return tvls[i].Epoch < tvls[j].Epoch
	})
	sort.Slice(filsupplys, func(i, j int) bool {
		return filsupplys[i].Id < filsupplys[j].Id
	})
	resp = &filscan.NetworkCapitalFigureReply{}
	//不一样的长度
	for i, j := 0, 0; i < len(filsupplys) && j < len(tvls); {
		if tvls[j].Epoch == filsupplys[i].Id-diff {
			var filMined, filVested, filReserveDisbursed, filBurnt, filCirculating, filLocked decimal.Decimal
			supply := filsupplys[i].CirculatingSupply
			filMined, err = decimal.NewFromString(supply.FilMined)
			if err != nil {
				return
			}
			filVested, err = decimal.NewFromString(supply.FilVested)
			if err != nil {
				return
			}
			filReserveDisbursed, err = decimal.NewFromString(supply.FilReserveDisbursed)
			if err != nil {
				return
			}
			filBurnt, err = decimal.NewFromString(supply.FilBurnt)
			if err != nil {
				return
			}
			filCirculating, err = decimal.NewFromString(supply.FilCirculating)
			if err != nil {
				return
			}
			filLocked, err = decimal.NewFromString(supply.FilLocked)
			if err != nil {
				return
			}

			point := &filscan.NetworkCapitalPoint{
				//这里epoch使用整点，即agg同步的时间
				Epoch:       filsupplys[i].Id,
				BlockTime:   chain.Epoch(filsupplys[i].Id).Unix(),
				Circulating: filCirculating,
				Produced:    filMined.Add(filVested).Add(filReserveDisbursed),
				Locked:      filLocked.Add(tvls[j].Tvl.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))), //todo defi
				Burn:        filBurnt,
			}
			resp.List = append(resp.List, point)
			j++
			i++
		} else if tvls[j].Epoch > filsupplys[i].Id-diff {
			i++
		} else {
			j++
		}
	}
	resp.Epoch = epoch
	resp.BlockTime = chain.Epoch(epoch).Unix()
	return resp, nil
}

func (r ResourceBiz) GetEventsList(ctx context.Context, _ struct{}) (*filscan.GetEventsListReply, error) {
	items, err := r.eventsRepo.GetEventsList(ctx)
	if err != nil {
		return nil, err
	}

	resp := &filscan.GetEventsListReply{}
	for i := range items {
		resp.Items = append(resp.Items, filscan.Events{
			ImageUrl: items[i].ImageUrl,
			JumpUrl:  items[i].JumpUrl,
			StartAt:  items[i].StartAt,
			EndAt:    items[i].EndAt,
			Name:     items[i].Name,
		})
	}
	return resp, nil
}

func (r ResourceBiz) GetFEvmHotItems(ctx context.Context, _ struct{}) (*filscan.GetFEvmItemsByCategoryReply, error) {
	items, categorys, err := r.repo.GetHotItems(ctx)
	if err != nil {
		return nil, err
	}

	sort.Slice(categorys, func(i, j int) bool {
		return categorys[i].Orders < categorys[j].Orders
	})

	mp := map[int]*po.FEvmItem{}
	for i := range items {
		mp[items[i].Id] = items[i]
	}
	reply := filscan.GetFEvmItemsByCategoryReply{}
	for i := range categorys {
		if _, ok := mp[categorys[i].ItemId]; !ok {
			log.Error("find categorys item failed", categorys[i].ItemId)
			continue
		}
		item := mp[categorys[i].ItemId]
		reply = append(reply, filscan.FEvmItem{
			Twitter:  item.Twitter,
			MainSite: item.MainSite,
			Name:     item.Name,
			Logo:     item.Logo,
			Category: categorys[i].Category,
		})
	}
	return &reply, nil
}

func (r ResourceBiz) GetFEvmItemsByCategory(ctx context.Context, req *filscan.GetFEvmItemsByCategoryReq) (*filscan.GetFEvmItemsByCategoryReply, error) {
	items, categorys, err := r.repo.GetFEvmItemsByCategory(ctx, req.Category)
	if err != nil {
		return nil, err
	}

	sort.Slice(categorys, func(i, j int) bool {
		return categorys[i].Orders < categorys[j].Orders
	})

	mp := map[int]*po.FEvmItem{}
	for i := range items {
		mp[items[i].Id] = items[i]
	}
	reply := filscan.GetFEvmItemsByCategoryReply{}
	for i := range categorys {
		if _, ok := mp[categorys[i].ItemId]; !ok {
			log.Error("find categorys item failed", categorys[i].ItemId)
			continue
		}
		item := mp[categorys[i].ItemId]
		reply = append(reply, filscan.FEvmItem{
			Twitter:  item.Twitter,
			MainSite: item.MainSite,
			Name:     item.Name,
			Logo:     item.Logo,
			Category: categorys[i].Category,
		})
	}
	return &reply, nil
}

func (r ResourceBiz) GetFEvmCategory(ctx context.Context, _ struct{}) (*filscan.GetFEvmCategoryReply, error) {
	label, count, err := r.repo.GetFEvmCategorys(ctx)
	if err != nil {
		return nil, err
	}
	if len(label) != len(count) {
		log.Error("fevm category count != count", label, count)
		return nil, nil
	}
	res := filscan.GetFEvmCategoryReply{}
	for i := range label {
		res = append(res, filscan.GetFEvmCategoryItem{
			Label: label[i],
			Num:   count[i],
		})
	}
	sort.Slice(res, func(i, j int) bool {
		return res[i].Num > res[j].Num
	})

	return &res, nil
}

func (r ResourceBiz) GetBannerList(ctx context.Context, req *filscan.GetBannerListReq) (*filscan.GetBannerListReply, error) {
	banner, err := r.repo.GetBannerByCategoryAndLanguage(ctx, req.Category, req.Language)
	if err != nil {
		return nil, err
	}
	sort.Slice(banner, func(i, j int) bool {
		return banner[i].Orders < banner[j].Orders
	})

	reply := &filscan.GetBannerListReply{}
	for i := range banner {
		reply.Items = append(reply.Items, filscan.BannerItem{
			Url:  banner[i].Url,
			Link: banner[i].Link,
		})
	}

	return reply, nil
}

var _ filscan.ResourceAPI = (*ResourceBiz)(nil)

func NewResourceBiz(db *gorm.DB, adapter londobell.Adapter, agg londobell.Agg) *ResourceBiz {

	VestReleaseDateRespLock = sync.RWMutex{}
	go func() {
		addrs := []po.ReleaseAddrs{}
		err := db.Find(&addrs).Error
		if err != nil {
			panic(err)
		}

		for {

			VestReleaseDateRespTmp := []filscan.ReleaseItem{}
			for i := range addrs {
				released := 0.0
				if chain.CalcEpochByTime(time.Now()).Int64() > addrs[i].EndEpoch {
					released = addrs[i].InitialLock
				} else {
					released = addrs[i].InitialLock * (float64((chain.CalcEpochByTime(time.Now()).Int64() - addrs[i].StartEpoch)) / (float64)(addrs[i].EndEpoch-addrs[i].StartEpoch))
				}
				actor, err := adapter.Actor(context.TODO(), chain.SmartAddress(addrs[i].Address), nil)
				if err != nil {
					continue
				}

				abd := dal.NewActorBalanceTrendBizDal(db)
				abt, err := abd.GetActorUnderEpochBalance(context.TODO(), chain.SmartAddress(addrs[i].Address), chain.CalcEpochByTime(time.Now())-2880*7)
				if err != nil {
					continue
				}

				balance7DayAgo := actor.Balance
				if abt != nil {
					balance7DayAgo = abt.Balance
				}

				tmp := filscan.ReleaseItem{
					AccountTag:      addrs[i].Tag,
					Released:        released,
					UnlockStartTime: chain.Epoch(addrs[i].StartEpoch).Unix(),
					AccountID:       addrs[i].Address,
					Balance:         actor.Balance.Div(decimal.New(1, 18)).InexactFloat64(),
					UnlockEndTime:   chain.Epoch(addrs[i].EndEpoch).Unix(),
					InitialBalance:  addrs[i].InitialLock,
					BalanceChanged:  actor.Balance.Sub(balance7DayAgo).Div(decimal.New(1, 18)).InexactFloat64(),
				}
				VestReleaseDateRespTmp = append(VestReleaseDateRespTmp, tmp)
			}
			VestReleaseDateRespLock.Lock()
			VestReleaseDateResp = VestReleaseDateRespTmp
			VestReleaseDateRespLock.Unlock()
			time.Sleep(time.Minute * 10)
		}

	}()
	return &ResourceBiz{
		db:         db,
		repo:       dal.NewResourceDal(db),
		eventsRepo: dal.NewEventsDal(db),
		adapter:    acl.NewStatisticAclImpl(adapter),
		filPrice:   dal.NewFilPriceDal(db),
		defiRepo:   dal.NewDefiDashboardDal(db, dal.NewERC20Dal(db)),
		syncEpoch:  dal.NewSyncerDal(db),
		agg:        agg,
	}
}
