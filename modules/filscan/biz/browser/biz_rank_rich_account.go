package browser

import (
	"context"
	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

func NewRichAccountRankBiz(adapter londobell.Adapter, db *gorm.DB) *RichAccountRankBiz {
	return &RichAccountRankBiz{
		adapter:          adapter,
		se:               dal.NewSyncEpochGetterDal(db),
		actorGetter:      dal.NewActorGetter(db),
		actorBalanceRank: dal.NewActorBalanceTaskDal(db),
	}
}

var _ filscan.RichAccountAPI = (*RichAccountRankBiz)(nil)

type RichAccountRankBiz struct {
	adapter          londobell.Adapter
	se               repository.SyncEpochGetter
	actorGetter      repository.ActorGetter
	actorBalanceRank repository.ActorBalanceTaskRepo
}

func (r RichAccountRankBiz) RichAccountRank(ctx context.Context, query filscan.PagingQuery) (resp filscan.RichAccountsResponse, err error) {
	err = query.Valid()
	if err != nil {
		return
	}
	richAccountList, err := r.actorBalanceRank.GetRichAccountRank(ctx, query)
	if err != nil {
		return
	}
	initialPledge, err := r.adapter.CurrentSectorInitialPledge(ctx, nil)
	if err != nil {
		return
	}

	////FIL的基础发放: 2,000,000,000 FIL
	//allBalance := decimal.NewFromFloat(2).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(9)))
	//// 转换单位为attoFil 用于计算
	//allBalance = allBalance.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	// FIL流通部分：2,000,000,000 FIL (基础发放) * 流通率
	currentCirculating := initialPledge.FilCirculating
	// FIL扇区抵押部分
	locked := initialPledge.FilLocked
	// FIL的保留部分: 300,000,000 FIL
	totalReserved := decimal.NewFromInt(3).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(8)))
	// 转换单位为attoFil 用于计算
	totalReservedAttoFil := totalReserved.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	// FIL剩余保留部分(f090余额)
	remainingVReserved := totalReservedAttoFil.Sub(initialPledge.FilReserveDisbursed)
	// 富豪榜总量 = FIL流通部分 + FIL扇区抵押部分 + FIL剩余保留部分
	totalAccountBalance := currentCirculating.Add(locked).Add(remainingVReserved)

	if richAccountList != nil {
		for _, richAccount := range richAccountList.RichAccountRankList {
			var actorInfo *bo.ActorInfo
			actorInfo, err = r.actorGetter.GetActorInfoByID(ctx, actor.Id(richAccount.Actor))
			if err != nil {
				return
			}
			var actorLastTxTime int64
			if actorInfo != nil && actorInfo.LastTxTime != nil {
				actorLastTxTime = actorInfo.LastTxTime.In(chain.TimeLoc).Unix()
			} else {
				info, err := GetInfoFromFilfox(ctx, richAccount.Actor)
				if err != nil {
					log.Errorf("get info from filfox failed: %w", err)
				} else {
					actorLastTxTime = info.LastSeen
				}
			}
			convertor := assembler.RichAccountRankAssembler{}
			var richAccountRank *filscan.RichAccount
			richAccountRank, err = convertor.ToRichAccountRank(richAccount, totalAccountBalance, actorLastTxTime)
			if err != nil {
				return
			}
			resp.GetRichAccountList = append(resp.GetRichAccountList, richAccountRank)
			resp.TotalCount = richAccountList.TotalCount
		}
	}

	return
}
