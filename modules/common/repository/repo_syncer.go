package repository

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type SyncerRepo interface {
	SaveSyncSyncerEpoch(ctx context.Context, item *po.SyncSyncerEpoch) (err error)
	GetSyncSyncerEpochOrNil(ctx context.Context, epoch chain.Epoch, name string) (item *po.SyncSyncerEpoch, err error)
	GetLatestSyncSyncerEpochOrNil(ctx context.Context, name string) (item *po.SyncSyncerEpoch, err error)
	GetSyncSyncerEpochs(ctx context.Context, name string, epochs chain.LCRCRange) (items []*po.SyncSyncerEpoch, err error)
	GetSyncTasksEpochs(ctx context.Context, names []string, epochs chain.LCRCRange) (items []*po.SyncTaskEpoch, err error)
	SaveSyncTaskEpochEpoch(ctx context.Context, item *po.SyncTaskEpoch) (err error)
	DeleteSyncTaskEpochs(ctx context.Context, gteEpoch chain.Epoch, syncer string) (err error)
	DeleteSyncSyncerEpochs(ctx context.Context, gteEpoch chain.Epoch, name string) (err error)
	GetSyncTaskEpochOrNil(ctx context.Context, epoch chain.Epoch, name string) (item *po.SyncTaskEpoch, err error)
	SaveSyncer(ctx context.Context, task *po.SyncSyncer) (err error)
	GetSyncerOrNil(ctx context.Context, name string) (task *po.SyncSyncer, err error)
	SyncerGetter
}

type SyncerGetter interface {
	GetSyncer(ctx context.Context, syncer string) (item *po.SyncSyncer, err error)
}
