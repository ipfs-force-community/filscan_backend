package biz

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/api"
)

func NewAdminBiz(manager *syncer.Manager) *AdminBiz {
	return &AdminBiz{manager: manager}
}

var _ syncer_api.AdminAPI = (*AdminBiz)(nil)

type AdminBiz struct {
	manager *syncer.Manager
}

func (a AdminBiz) SyncerEpochs(ctx context.Context) (epochs []*syncer_api.SyncerEpoch, err error) {
	r := a.manager.SyncerHeights()
	for k, v := range r {
		epochs = append(epochs, &syncer_api.SyncerEpoch{
			Syncer: k,
			Epoch:  v.Int64(),
			Time:   v.Time(),
		})
	}
	return
}
