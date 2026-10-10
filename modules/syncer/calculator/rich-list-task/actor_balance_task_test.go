package rich_list_task_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/gozelle/mix"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	rich_list_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/rich-list-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
)

// 本文件用假聚合器 + 假仓储覆盖 rich-list-task 的核心修复：
// 「游标越过当天边界即补写、且同一期只写一次」——不连真库、不连真网。

// ---- 假聚合器：只覆盖 RichList，其余由嵌入接口兜底 ----

type fakeAgg struct {
	londobell.Agg

	list *londobell.RichList
	err  error

	calls       int
	lastFilters types.Filters
}

func (f *fakeAgg) RichList(_ context.Context, filters types.Filters) (*londobell.RichList, error) {
	f.calls++
	f.lastFilters = filters
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

// ---- 假仓储：模拟 chain.rich_actors 上的「本期是否已写入」语义 ----

type fakeRepo struct {
	mu sync.Mutex

	exists map[int64]bool

	saveCalls  int
	savedEpoch []int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{exists: map[int64]bool{}}
}

func (r *fakeRepo) GetRichAccountRank(context.Context, filscan.PagingQuery) (*bo.RichAccountRankList, error) {
	return nil, nil
}

func (r *fakeRepo) SaveActorsBalance(_ context.Context, items []*actor.RichActor) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveCalls++
	if len(items) > 0 {
		// 落库后本期即「已存在」（对应真实表的 (epoch, actor_id) 唯一索引）
		r.exists[items[0].Epoch] = true
		r.savedEpoch = append(r.savedEpoch, items[0].Epoch)
	}
	return nil
}

func (r *fakeRepo) DeleteActorsBalance(_ context.Context, _ chain.Epoch) error { return nil }

func (r *fakeRepo) ExistsActorsBalance(_ context.Context, epoch chain.Epoch) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.exists[epoch.Int64()], nil
}

func sampleList() *londobell.RichList {
	return &londobell.RichList{
		TotalCount: 1,
		RichList: []*londobell.RichInfo{
			{Addr: chain.SmartAddress("f0100"), Balance: decimal.NewFromInt(123), Code: types.ACCOUNT},
		},
	}
}

func TestCalc_DayBoundary(t *testing.T) {
	// 当天起始高度由本机时钟算出：与 Calc 内部同一算法，逐字对齐。
	currentDay := chain.CurrentEpoch().CurrentDay()

	t.Run("游标未到当天边界：不取榜、不写", func(t *testing.T) {
		agg := &fakeAgg{list: sampleList()}
		repo := newFakeRepo()
		task := rich_list_task.NewActorBalanceTask(nil, repo)

		err := task.Calc(syncer.NewTestContext(nil, agg, currentDay-1))
		require.NoError(t, err)
		require.Equal(t, 0, agg.calls, "游标未到边界不应请求聚合器")
		require.Equal(t, 0, repo.saveCalls, "游标未到边界不应写库")
	})

	t.Run("游标已越过边界且本期不存在：补写 epoch=当天边界", func(t *testing.T) {
		agg := &fakeAgg{list: sampleList()}
		repo := newFakeRepo()
		task := rich_list_task.NewActorBalanceTask(nil, repo)

		err := task.Calc(syncer.NewTestContext(nil, agg, currentDay+1000))
		require.NoError(t, err)
		require.Equal(t, 1, agg.calls)
		require.Equal(t, 1, repo.saveCalls)
		require.Equal(t, []int64{currentDay.Int64()}, repo.savedEpoch, "写入的 epoch 必须是当天边界，而非当前游标高度")
		require.NotNil(t, agg.lastFilters.Start, "取榜必须带 start")
		require.Equal(t, currentDay.Int64(), agg.lastFilters.Start.Int64())
		require.Equal(t, int64(1000), agg.lastFilters.Limit, "仍取前 1000 名")
	})

	t.Run("本期已存在：幂等跳过，不重复写、不取榜", func(t *testing.T) {
		agg := &fakeAgg{list: sampleList()}
		repo := newFakeRepo()
		repo.exists[currentDay.Int64()] = true
		task := rich_list_task.NewActorBalanceTask(nil, repo)

		err := task.Calc(syncer.NewTestContext(nil, agg, currentDay+1000))
		require.NoError(t, err)
		require.Equal(t, 0, agg.calls, "本期已写入不应再请求聚合器")
		require.Equal(t, 0, repo.saveCalls, "本期已写入不应重复写库")
	})

	t.Run("越过边界后连续多个高度只写一次", func(t *testing.T) {
		agg := &fakeAgg{list: sampleList()}
		repo := newFakeRepo()
		task := rich_list_task.NewActorBalanceTask(nil, repo)

		for _, e := range []chain.Epoch{currentDay, currentDay + 1, currentDay + 120, currentDay + 2880} {
			require.NoError(t, task.Calc(syncer.NewTestContext(nil, agg, e)))
		}
		require.Equal(t, 1, repo.saveCalls, "同一期只应写入一次（幂等）")
		require.Equal(t, []int64{currentDay.Int64()}, repo.savedEpoch)
	})

	t.Run("富豪榜为空：只 warn 不写", func(t *testing.T) {
		agg := &fakeAgg{list: &londobell.RichList{TotalCount: 0}}
		repo := newFakeRepo()
		task := rich_list_task.NewActorBalanceTask(nil, repo)

		err := task.Calc(syncer.NewTestContext(nil, agg, currentDay+1))
		require.Error(t, err)
		var warn *mix.Warn
		require.True(t, errors.As(err, &warn), "富豪榜为空应返回 *mix.Warn（可重试、不算坏点）")
		require.Equal(t, 0, repo.saveCalls, "富豪榜为空不应写库")
	})

	t.Run("空高度：直接返回，什么都不做", func(t *testing.T) {
		agg := &fakeAgg{list: sampleList()}
		repo := newFakeRepo()
		task := rich_list_task.NewActorBalanceTask(nil, repo)

		ctx, err := syncer.NewTestContextWithData(nil, agg, currentDay+1, true, nil)
		require.NoError(t, err)
		require.NoError(t, task.Calc(ctx))
		require.Equal(t, 0, agg.calls)
		require.Equal(t, 0, repo.saveCalls)
	})
}
