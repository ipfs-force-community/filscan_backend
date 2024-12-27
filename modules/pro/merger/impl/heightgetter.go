package mergerimpl

import (
	"context"
	"fmt"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type heightGetter struct {
	agg     londobell.Agg
	adapter londobell.Adapter
}

func (h heightGetter) TraceHeight(ctx context.Context) (epoch chain.Epoch, err error) {
	
	r, err := h.agg.FinalHeight(ctx)
	if err != nil {
		err = fmt.Errorf("get final height error: %w", err)
		return
	}
	
	if r == nil {
		err = fmt.Errorf("final height is nil")
		return
	}
	
	epoch = *r
	
	return
}

func (h heightGetter) StateHeight(ctx context.Context) (epoch chain.Epoch, err error) {
	r, err := h.agg.StateFinalHeight(ctx)
	if err != nil {
		return
	}
	epoch = *r
	return
}

func (h heightGetter) AdapterHeight(ctx context.Context) (epoch chain.Epoch, err error) {
	
	r, err := h.adapter.Epoch(ctx, nil)
	if err != nil {
		return
	}
	epoch = chain.Epoch(r.Epoch)
	return
}
