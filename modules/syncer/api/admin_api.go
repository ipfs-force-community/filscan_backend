package syncer_api

import (
	"context"
	"time"
)

type AdminAPI interface {
	SyncerEpochs(ctx context.Context) (epochs []*SyncerEpoch, err error)
}

type SyncerEpoch struct {
	Syncer string
	Epoch  int64
	Time   time.Time
}
