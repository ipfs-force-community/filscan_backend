package impl

import (
	"context"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"testing"
)

func TestNewMinerAggImpl(t *testing.T) {
	
	m, err := NewMinerAggImpl("http://192.168.1.57:3000/mock/19")
	require.NoError(t, err)
	
	r, err := m.PeriodBlockRewards(context.Background(), chain.SmartAddress("f02099116"), chain.NewLCRORange(3087732, 3087733))
	require.NoError(t, err)
	
	spew.Json(r)
}
