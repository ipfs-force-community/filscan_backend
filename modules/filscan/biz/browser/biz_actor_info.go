package browser

import (
	"context"
	"math/big"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

func NewActorInfoBizBiz(db *gorm.DB) *ActorInfoBiz {
	return &ActorInfoBiz{
		db:           db,
		actorInfo:    dal.NewActorGetter(db),
		balanceTrend: dal.NewActorBalanceTrendBizDal(db),
		syncEpoch:    dal.NewSyncerDal(db),
	}
}

type ActorInfoBiz struct {
	db           *gorm.DB
	actorInfo    repository.ActorGetter
	balanceTrend repository.ActorBalanceTrendBizRepo
	syncEpoch    repository.SyncerGetter
}

func (a ActorInfoBiz) GetActorInfoOrNil(ctx context.Context, accountID chain.SmartAddress) (info *bo.ActorInfo, err error) {
	info, err = a.actorInfo.GetActorInfoByID(ctx, accountID)
	if err != nil {
		return
	}
	return
}

func (a ActorInfoBiz) GetActorBalanceTrend(ctx context.Context, accountBasic *filscan.AccountBasic, accountInterval types.IntervalType, epoch *int64) ([]*filscan.BalanceTrend, error) {
	if epoch == nil {
		tmp := chain.CurrentEpoch().Int64()
		epoch = &tmp
	}

	resolveInterval, err := interval.ResolveInterval2(string(accountInterval), chain.Epoch(*epoch))
	if err != nil {
		return nil, err
	}
	rp := resolveInterval.Points()
	actorBalanceDal := dal.NewActorBalanceTrendBizDal(a.db)
	var accountID actor.Id
	var currentBalance decimal.Decimal
	createdEpoch := chain.Epoch(0)

	if accountBasic != nil {
		accountID = actor.Id(accountBasic.AccountID)
		currentBalance = accountBasic.AccountBalance
		if accountBasic.CreateTime != nil {
			createdEpoch = chain.CalcEpochByTime(time.Unix(*accountBasic.CreateTime, 0))
		}
	}

	var actorBalanceTrend []*bo.ActorBalanceTrend
	actorBalanceTrend, err = actorBalanceDal.GetActorBalanceTrend(ctx, accountID, resolveInterval.Start(), rp)
	if err != nil {
		return nil, err
	}

	// 如果创建时间不在区间里则需要去寻找上一个变化点
	// 否则创建时间会是起始序列，且数据都要向他靠
	actorBalanceFirst, err := actorBalanceDal.GetActorUnderEpochBalance(ctx, accountID, resolveInterval.Start())
	if err != nil {
		return nil, err
	}

	// 如果上一个点存在 则直接放入
	// 否则 如果变化序列存在，则放入最靠近上一个点的点的信息（即数组的最后一个）
	// 如果 变化序列也是空的 意味着这段时间一直没变化，则放入当前状态
	if actorBalanceFirst != nil {
		actorBalanceTrend = append(actorBalanceTrend, actorBalanceFirst)
	} else if createdEpoch > resolveInterval.Start() {
		actorBalanceTrend = append(actorBalanceTrend, &bo.ActorBalanceTrend{
			Epoch:             0,
			AccountID:         "",
			Balance:           decimal.Zero,
			AvailableBalance:  &decimal.Zero,
			InitialPledge:     &decimal.Zero,
			PreCommitDeposits: &decimal.Zero,
			LockedBalance:     &decimal.Zero,
		})
	} else if len(actorBalanceTrend) != 0 {
		tmp := *actorBalanceTrend[len(actorBalanceTrend)-1]
		tmp.Epoch = 0
		actorBalanceTrend = append(actorBalanceTrend, &tmp)
	} else {
		actorBalanceTrend = append(actorBalanceTrend, &bo.ActorBalanceTrend{
			Epoch:             0,
			AccountID:         "",
			Balance:           currentBalance,
			AvailableBalance:  &decimal.Zero,
			InitialPledge:     &decimal.Zero,
			PreCommitDeposits: &decimal.Zero,
			LockedBalance:     &decimal.Zero,
		})
	}

	currentActor := &filscan.BalanceTrend{
		Height:    big.NewInt(*epoch),
		BlockTime: chain.Epoch(*epoch).Unix(),
		Balance:   currentBalance,
	}
	var balanceTrends []*filscan.BalanceTrend
	convert := assembler.ActorInfo{}
	balanceTrends = convert.ToActorBalanceTrendResponse(actorBalanceTrend, currentActor)
	balanceTrends, err = a.findNearestPoints(balanceTrends, rp, createdEpoch)
	if err != nil {
		return nil, err
	}

	return balanceTrends, nil
}

// FindNearestPoints
// points是以当前高度为起始，根据输入的interval往前找每个小时的点，即我们所要展示的点的列表，排序为按高度从小到大排列；
// input是根据change actor的同步任务所统计的点，若没有余额变化，则至少会有两个点（interval起始点和interval结束点）
// createdEpoch是actor的创建时间epoch
func (a ActorInfoBiz) findNearestPoints(inputs []*filscan.BalanceTrend, points []chain.Epoch, createdEpoch chain.Epoch) (result []*filscan.BalanceTrend, err error) {
	// points是以syncer同步器当前高度为起始，根据输入的interval往前找每个小时的点，即我们所要展示的点的列表，排序为按高度从小到大排列；
	for i := 0; i < len(points); i++ {
		// 如果存在inputs的点在两个points之间，则此点的余额等信息为下一个point的点的信息
		var newBalance *filscan.BalanceTrend
		for j := range inputs {
			if inputs[j].Height.Int64() >= points[i].Int64() && inputs[j+1].Height.Int64() <= points[i].Int64() {
				if inputs[j].Height.Int64() == points[i].Int64() {
					newBalance = &filscan.BalanceTrend{
						Height:            big.NewInt(points[i].Int64()),
						BlockTime:         points[i].Unix(),
						Balance:           inputs[j].Balance,
						AvailableBalance:  inputs[j].AvailableBalance,
						InitialPledge:     inputs[j].InitialPledge,
						LockedFunds:       inputs[j].LockedFunds,
						PreCommitDeposits: inputs[j].PreCommitDeposits,
					}
				} else {
					newBalance = &filscan.BalanceTrend{
						Height:            big.NewInt(points[i].Int64()),
						BlockTime:         points[i].Unix(),
						Balance:           inputs[j+1].Balance,
						AvailableBalance:  inputs[j+1].AvailableBalance,
						InitialPledge:     inputs[j+1].InitialPledge,
						LockedFunds:       inputs[j+1].LockedFunds,
						PreCommitDeposits: inputs[j+1].PreCommitDeposits,
					}
				}
			}
		}
		if newBalance != nil {
			result = append(result, newBalance)
		} else {
			result = append(result, inputs[0])
		}

	}

	return
}
