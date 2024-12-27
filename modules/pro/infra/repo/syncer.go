package prorepo

import (
	"context"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type SyncerRepo interface {
	MinerEpoch(ctx context.Context) (epoch chain.Epoch, err error)
	TraceEpoch(ctx context.Context) (epoch chain.Epoch, err error)
	SectorEpoch(ctx context.Context) (epoch chain.Epoch, err error)
	ChangeActorEpoch(ctx context.Context) (epoch chain.Epoch, err error)
}
