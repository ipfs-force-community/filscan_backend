package impl

import (
	"context"
	"github.com/go-resty/resty/v2"
	"github.com/gozelle/spew"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestGetChangeActors(t *testing.T) {
	ad := NewLondobellAdapterImpl("http://106.14.10.70:12345", resty.New())
	res, err := ad.ChangeActors(context.Background(), 2797694)
	require.NoError(t, err)
	spew.Json(res)
}
