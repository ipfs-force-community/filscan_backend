package browser

import (
	"context"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/assembler"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/interval"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/gorm"
)

func NewOwnerInfoBiz(db *gorm.DB) *OwnerInfoBiz {
	return &OwnerInfoBiz{
		db: db,
		se: dal.NewSyncEpochGetterDal(db),
	}
}

type OwnerInfoBiz struct {
	db *gorm.DB
	se repository.SyncEpochGetter
}

func (o OwnerInfoBiz) CheckIsOwner(ctx context.Context, addr chain.SmartAddress) (result bool, err error) {
	checker := dal.NewOwnerGetterDal(o.db)
	ok, err := checker.IsOwner(ctx, addr)
	if err != nil {
		return
	}
	result = ok
	return
}

func (o OwnerInfoBiz) GetOwnerInfo(ctx context.Context, addr chain.SmartAddress) (result *filscan.AccountOwner, err error) {
	ownerInfoDal := dal.NewOwnerInfoBizDal(o.db)
	ownerInfo, err := ownerInfoDal.GetOwnerInfo(ctx, addr)
	if err != nil {
		return
	}
	if ownerInfo != nil {
		converter := assembler.OwnerInfoAssembler{}
		var newOwnerInfo *filscan.AccountOwner
		newOwnerInfo, err = converter.ToOwnerInfoResponse(ownerInfo)
		if err != nil {
			return
		}
		if ownerInfo.Miners != nil {
			newOwnerInfo.OwnedMiners = ownerInfo.Miners
		}
		result = newOwnerInfo
	}

	return
}

func (o OwnerInfoBiz) GetOwnerIndicator(ctx context.Context, addr chain.SmartAddress, interval *types.IntervalType) (result *filscan.MinerIndicators, err error) {
	ownerIndicatorDal := dal.NewOwnerIndicatorDal(o.db)
	ownerIndicator, err := ownerIndicatorDal.GetOwnerIndicator(ctx, addr, interval)
	if err != nil {
		return
	}
	var powerRatio decimal.Decimal
	var sectorRatio decimal.Decimal
	switch interval.Value() {
	case types.DAY:
		powerRatio = ownerIndicator.QualityAdjPowerChange.Div(decimal.NewFromInt(1))
		sectorRatio = ownerIndicator.SealPowerChange.Div(decimal.NewFromInt(1))
	case types.WEEK:
		powerRatio = ownerIndicator.QualityAdjPowerChange.Div(decimal.NewFromInt(7))
		sectorRatio = ownerIndicator.SealPowerChange.Div(decimal.NewFromInt(7))
	case types.MONTH:
		powerRatio = ownerIndicator.QualityAdjPowerChange.Div(decimal.NewFromInt(30))
		sectorRatio = ownerIndicator.SealPowerChange.Div(decimal.NewFromInt(30))
	case types.YEAR:
		powerRatio = ownerIndicator.QualityAdjPowerChange.Div(decimal.NewFromInt(365))
		sectorRatio = ownerIndicator.SealPowerChange.Div(decimal.NewFromInt(365))
	}
	converter := assembler.OwnerInfoAssembler{}
	newOwnerIndicator, err := converter.ToOwnerIndicatorResponse(ownerIndicator, powerRatio, sectorRatio)
	if err != nil {
		return
	}
	result = newOwnerIndicator
	return
}

func (o OwnerInfoBiz) OwnerBalanceTrend(ctx context.Context, accountID chain.SmartAddress, accountInterval types.IntervalType) (balanceTrends []*filscan.BalanceTrend, err error) {
	epoch, err := o.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	if epoch != nil {
		var resolveInterval interval.Interval
		resolveInterval, err = interval.ResolveInterval(string(accountInterval), *epoch)
		if err != nil {
			return
		}
		ownerBalanceDal := dal.NewOwnerBalanceTrendBizDal(o.db)
		var ownerBalanceTrend []*bo.ActorBalanceTrend
		ownerBalanceTrend, err = ownerBalanceDal.GetOwnerBalanceTrend(ctx, resolveInterval.Points(), accountID)
		if err != nil {
			return
		}
		converter := assembler.OwnerInfoAssembler{}
		var newOwnerBalanceTrends []*filscan.BalanceTrend
		newOwnerBalanceTrends, err = converter.ToOwnerBalanceTrendResponse(ownerBalanceTrend)
		if err != nil {
			return
		}
		balanceTrends = newOwnerBalanceTrends
	}
	return
}

func (o OwnerInfoBiz) OwnerPowerTrend(ctx context.Context, accountID chain.SmartAddress, accountInterval types.IntervalType) (powerTrends []*filscan.PowerTrend, err error) {
	epoch, err := o.se.MinerEpoch(ctx)
	if err != nil {
		return
	}
	if epoch != nil {
		var resolveInterval interval.Interval
		resolveInterval, err = interval.ResolveInterval(string(accountInterval), *epoch)
		if err != nil {
			return
		}
		ownerPowerDal := dal.NewOwnerPowerTrendBizDal(o.db)
		var ownerPowerTrend []*bo.ActorPowerTrend
		ownerPowerTrend, err = ownerPowerDal.GetOwnerPowerTrend(ctx, resolveInterval.Points(), accountID)
		if err != nil {
			return
		}
		converter := assembler.OwnerInfoAssembler{}
		var newOwnerPowerTrends []*filscan.PowerTrend
		newOwnerPowerTrends, err = converter.ToOwnerPowerTrendResponse(ownerPowerTrend)
		if err != nil {
			return
		}
		powerTrends = newOwnerPowerTrends
	}
	return
}
