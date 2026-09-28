package metrics

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type countingQuerier struct {
	fake  *fakeQuerier
	heads int
}

func (q *countingQuerier) HeadHeight(ctx context.Context) (int64, time.Time, error) {
	q.heads++
	return q.fake.HeadHeight(ctx)
}

func (q *countingQuerier) Syncers(ctx context.Context) ([]SyncerCursor, error) {
	return q.fake.Syncers(ctx)
}

func (q *countingQuerier) MaxHeight(ctx context.Context, spec TableSpec) (int64, bool, error) {
	return q.fake.MaxHeight(ctx, spec)
}

func TestServiceCachesWithinTTL(t *testing.T) {
	f := newFakeQuerier(1000)
	f.cursors = []SyncerCursor{{Name: "chain", Epoch: 990}}
	q := &countingQuerier{fake: f}
	c := NewCollector(q, "mainnet", specsOf(t, "chain.actor_actions:epoch"))
	s := NewService(c, time.Minute, log.New(io.Discard, "", 0))

	first := s.Render(context.Background(), false)
	second := s.Render(context.Background(), false)
	require.NotEmpty(t, first)
	require.Equal(t, string(first), string(second))
	require.Equal(t, 1, q.heads, "TTL 内不应重复打数据源")

	forced := s.Render(context.Background(), true)
	require.NotEmpty(t, forced)
	require.Equal(t, 2, q.heads, "force=true 必须重新采集")
}

func TestServiceMetricsHandler(t *testing.T) {
	f := newFakeQuerier(1000)
	f.cursors = []SyncerCursor{{Name: "chain", Epoch: 995}}
	f.max = map[string]int64{"chain.actor_actions:epoch": 990}
	q := &countingQuerier{fake: f}
	c := NewCollector(q, "mainnet", specsOf(t, "chain.actor_actions:epoch"))
	s := NewService(c, time.Minute, log.New(io.Discard, "", 0))

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Header().Get("Content-Type"), "text/plain")
	body := rec.Body.String()
	require.Contains(t, body, "# HELP filscan_syncer_lag_height")
	require.Contains(t, body, `filscan_syncer_lag_height{network="mainnet",syncer="chain"} 5`)
	require.Contains(t, body, "# HELP filscan_table_lag_height")
	require.Contains(t, body,
		`filscan_table_lag_height{column="epoch",network="mainnet",table="chain.actor_actions"} 10`)
	require.Contains(t, body, "# HELP filscan_metrics_up")
}

func TestServiceZeroTTLFallsBackToDefault(t *testing.T) {
	s := NewService(NewCollector(newFakeQuerier(1), "mainnet", nil), 0, nil)
	require.Equal(t, DefaultTTL, s.ttl)
	require.NotNil(t, s.logger)
}

func TestServiceRenderNeverPanicsOnEmptyCollector(t *testing.T) {
	// 链头不可用 + 表全失败：仍应得到一份合法（非 nil）的指标文本。
	f := newFakeQuerier(0)
	f.headErr = context.DeadlineExceeded
	f.syncersErr = context.DeadlineExceeded
	f.maxErr["chain.actor_actions:epoch"] = errors.New("relation does not exist")
	c := NewCollector(f, "mainnet", specsOf(t, "chain.actor_actions:epoch"))
	s := NewService(c, time.Minute, log.New(io.Discard, "", 0))

	body := string(s.Render(context.Background(), true))
	require.True(t, strings.HasPrefix(body, "# HELP filscan_metrics_up"))
	require.Contains(t, body, "filscan_metrics_up{network=\"mainnet\"} 0")
}
