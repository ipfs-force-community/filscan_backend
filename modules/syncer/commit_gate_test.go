package syncer

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// fakeGateAgg 只实现水位门槛用到的聚合器方法（LatestTipset 由别的测试覆盖，这里只测 FinalHeight）。
type fakeGateAgg struct {
	londobell.Agg // 嵌入接口：未用到的上百个方法不实现
	committed     *chain.Epoch
	err           error
	calls         int
}

func (f *fakeGateAgg) FinalHeight(_ context.Context) (*chain.Epoch, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.committed, nil
}

func newGateTestSyncer(gate bool, agg londobell.Agg) *Syncer {
	s := &Syncer{
		config: &config{
			name:             "test-gate",
			commitHeightGate: gate,
		},
		epoch: chain.Epoch(100),
	}
	s.agg = agg
	return s
}

func epochPtr(v int64) *chain.Epoch {
	e := chain.Epoch(v)
	return &e
}

// 关闭开关（默认）：门槛就是近链头水位，且不应去问正式水位（省一次聚合器调用）。
func TestCommitGatedHeight_GateOff(t *testing.T) {
	agg := &fakeGateAgg{committed: epochPtr(900)}
	s := newGateTestSyncer(false, agg)

	got, err := s.commitGatedHeight(chain.Epoch(1000))
	require.NoError(t, err)
	require.Equal(t, chain.Epoch(1000), got)
	require.Equal(t, 0, agg.calls, "关闭开关时不应查询正式提交水位")
}

// 开启开关且正式水位更低：取正式水位（这就是「等正式库的事件文档落库」的那道闸）。
func TestCommitGatedHeight_CommitLower(t *testing.T) {
	agg := &fakeGateAgg{committed: epochPtr(978)}
	s := newGateTestSyncer(true, agg)

	got, err := s.commitGatedHeight(chain.Epoch(1000))
	require.NoError(t, err)
	require.Equal(t, chain.Epoch(978), got)
	require.Equal(t, 1, agg.calls)
}

// 开启开关但正式水位已追上（或超前）：保持近链头水位，不做无谓等待。
func TestCommitGatedHeight_CommitNotLower(t *testing.T) {
	for _, committed := range []int64{1000, 1001} {
		agg := &fakeGateAgg{committed: epochPtr(committed)}
		s := newGateTestSyncer(true, agg)

		got, err := s.commitGatedHeight(chain.Epoch(1000))
		require.NoError(t, err)
		require.Equal(t, chain.Epoch(1000), got)
	}
}

// 正式水位取不到：返回错误（调用方按「未就绪」等下一轮），绝不退化成「近链头水位」继续消费
// —— 那正是会读到空回执、静默丢整高度行的老行为。
func TestCommitGatedHeight_ProbeError(t *testing.T) {
	agg := &fakeGateAgg{err: errors.New("dial tcp 172.31.38.30:1237: i/o timeout")}
	s := newGateTestSyncer(true, agg)

	got, err := s.commitGatedHeight(chain.Epoch(1000))
	require.Error(t, err)
	require.Equal(t, chain.Epoch(0), got)
}

// 正式水位取到 nil：同样按未就绪处理，不返回门槛。
func TestCommitGatedHeight_EmptyResult(t *testing.T) {
	agg := &fakeGateAgg{committed: nil}
	s := newGateTestSyncer(true, agg)

	_, err := s.commitGatedHeight(chain.Epoch(1000))
	require.Error(t, err)
}
