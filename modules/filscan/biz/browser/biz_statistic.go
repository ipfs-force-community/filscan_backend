package browser

import (
	"context"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/acl"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gorm.io/gorm"
)

func NewStatisticBiz(agg londobell.Agg, db *gorm.DB, adapter londobell.Adapter, conf *config.Config) *StatisticBiz {
	statisticAcl := acl.NewStatisticAclImpl(adapter)
	return &StatisticBiz{
		StatisticBaseLineBiz:           NewStatisticBaseLineBiz(dal.NewSyncerDal(db), dal.NewStatisticBaseLineBizDal(db), dal.NewMinerGetterDal(db), adapter),
		BaseFeeTrendBiz:                NewBaseFeeTrendBiz(dal.NewSyncerDal(db), dal.NewBaseFeeTrendBizDal(db)),
		ContractTrendBiz:               NewContractTrendBiz(dal.NewSyncerDal(db), dal.NewContractTrendBizDal(db), conf),
		StatisticActiveMinerTrendBiz:   NewStatisticActiveMinerTrendBiz(dal.NewSyncEpochGetterDal(db), dal.NewStatisticActiveMinerTrendBizDal(db)),
		StatisticBlockRewardTrendBiz:   NewStatisticBlockRewardTrendBiz(dal.NewSyncerDal(db), dal.NewStatisticBlockRewardTrendBizDal(db)),
		StatisticMessageCountTrendBiz:  NewStatisticMessageCountTrendBiz(dal.NewSyncerDal(db), dal.NewStatisticMessageCountTrendBizDal(db)),
		StatisticGasDataTrend:          NewStatisticGasDataTrendBiz(dal.NewGas24hTrendBizDal(db)),
		StatisticDcTrendBiz:            NewStatisticDcTrendBiz(dal.NewSyncerDal(db), dal.NewDcTrendDal(db)),
		StatisticRewardStreamsBiz:      NewStatisticRewardStreamsBiz(dal.NewSyncerDal(db), agg),
		StatisticRewardStreamLedgerBiz: NewStatisticRewardStreamLedgerBiz(dal.NewSyncerDal(db), statisticAcl, dal.NewRewardStreamRecipientDal(db)),
		adapter:                        statisticAcl,
	}
}

var _ filscan.StatisticAPI = (*StatisticBiz)(nil)

type StatisticBiz struct {
	*StatisticBaseLineBiz
	*BaseFeeTrendBiz
	*StatisticActiveMinerTrendBiz
	*StatisticBlockRewardTrendBiz
	*StatisticMessageCountTrendBiz
	*StatisticGasDataTrend
	*StatisticDcTrendBiz
	*StatisticRewardStreamsBiz
	*StatisticRewardStreamLedgerBiz
	*ContractTrendBiz
	adapter *acl.StatisticAclImpl
}

func (s StatisticBiz) FilCompose(ctx context.Context, req filscan.FilComposeRequest) (resp filscan.FilComposeResponse, err error) {
	filCompose, err := s.adapter.GetFilCompose(ctx)
	if err != nil {
		return
	}
	resp.FilCompose = filCompose
	return
}

func (s StatisticBiz) PeerMap(ctx context.Context, req filscan.PeerMapRequest) (resp filscan.PeerMapResponse, err error) {
	//TODO implement me
	panic("implement me")
}
