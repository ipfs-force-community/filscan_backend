package calc_reward_stream_recipient_task_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	calc_reward_stream_recipient_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-reward-stream-recipient-task"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/fevm/offline-replay"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件用假适配器 + 假仓储覆盖计算器口径与幂等/回滚边界，**不连真库、不连真网**。
// 另有两例用 offline-replay 的记录型假连接（不拨号）断言 dal 真正下发的 SQL：
// upsert 的 ON CONFLICT，以及删除/查询的 epoch 边界方向。

// ---- 假适配器：只需实现 RewardStreamLedger（其余由嵌入接口兜底）----

type fakeLedgerAdapter struct {
	londobell.Adapter

	ledger *londobell.RewardStreamLedger
	err    error

	requested []int64
}

func (f *fakeLedgerAdapter) RewardStreamLedger(_ context.Context, epoch *chain.Epoch) (*londobell.RewardStreamLedger, error) {
	if epoch != nil {
		f.requested = append(f.requested, epoch.Int64())
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.ledger, nil
}

// ---- 假仓储：实现 (epoch, address) 上的 upsert 语义（模拟唯一键 + ON CONFLICT）----

type fakeRecipientRepo struct {
	mu    sync.Mutex
	rows  map[string]*po.RewardStreamRecipientEpoch
	order []string

	saveCalls int
	gteArgs   []int64
	lteArgs   []int64
}

func newFakeRecipientRepo() *fakeRecipientRepo {
	return &fakeRecipientRepo{rows: map[string]*po.RewardStreamRecipientEpoch{}}
}

func recipKey(epoch int64, addr string) string { return fmt.Sprintf("%d|%s", epoch, addr) }

func (r *fakeRecipientRepo) SaveRewardStreamRecipients(_ context.Context, items []*po.RewardStreamRecipientEpoch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saveCalls++
	for _, it := range items {
		k := recipKey(it.Epoch, it.Address)
		if _, ok := r.rows[k]; !ok {
			r.order = append(r.order, k)
		}
		cp := *it // 覆盖：同一 (epoch,address) 只留一行
		r.rows[k] = &cp
	}
	return nil
}

func (r *fakeRecipientRepo) ListRewardStreamRecipientsByEpochRange(context.Context, chain.LCRCRange) ([]*po.RewardStreamRecipientEpoch, error) {
	return nil, nil
}

func (r *fakeRecipientRepo) DeleteRewardStreamRecipientsGteEpoch(_ context.Context, gteEpoch chain.Epoch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gteArgs = append(r.gteArgs, gteEpoch.Int64())
	return nil
}

func (r *fakeRecipientRepo) DeleteRewardStreamRecipientsLteEpoch(_ context.Context, lteEpoch chain.Epoch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lteArgs = append(r.lteArgs, lteEpoch.Int64())
	return nil
}

func (r *fakeRecipientRepo) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

func (r *fakeRecipientRepo) get(epoch int64, addr string) *po.RewardStreamRecipientEpoch {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rows[recipKey(epoch, addr)]
}

// ---- 构造 helper ----

func mkLedger(epoch int64, streams []*londobell.RewardStreamLedgerStream, tombs []*londobell.RewardStreamLedgerTombstone) *londobell.RewardStreamLedger {
	return &londobell.RewardStreamLedger{
		Epoch:      epoch,
		Nv29:       true,
		Denom:      decimal.NewFromInt(1_000_000_000_000_000_000),
		Streams:    streams,
		Tombstones: tombs,
	}
}

func mkStream(id uint64, implicit bool, recs ...*londobell.RewardStreamLedgerRecipient) *londobell.RewardStreamLedgerStream {
	return &londobell.RewardStreamLedgerStream{ID: id, Implicit: implicit, Recipients: recs}
}

func mkRec(addr string, share, payable, claimed int64) *londobell.RewardStreamLedgerRecipient {
	return &londobell.RewardStreamLedgerRecipient{
		Address:       addr,
		Share:         decimal.NewFromInt(share),
		Payable:       decimal.NewFromInt(payable),
		ClaimedPeriod: decimal.NewFromInt(claimed),
	}
}

func mkTomb(id uint64, recs ...*londobell.RewardStreamLedgerTombstoneRecipient) *londobell.RewardStreamLedgerTombstone {
	return &londobell.RewardStreamLedgerTombstone{ID: id, Recipients: recs}
}

func mkTombRec(addr string, payable int64) *londobell.RewardStreamLedgerTombstoneRecipient {
	return &londobell.RewardStreamLedgerTombstoneRecipient{Address: addr, Payable: decimal.NewFromInt(payable)}
}

func runCalc(t *testing.T, repo *fakeRecipientRepo, adapter *fakeLedgerAdapter, epoch chain.Epoch) {
	t.Helper()
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTask(repo)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, epoch)))
}

// ① 同高度重跑不产生重复行（幂等 upsert）：同一 (epoch,address) 覆盖，不是追加。
func TestCalcIdempotentRerunSameEpoch(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 100, 5, 1))}, nil)}

	runCalc(t, repo, adapter, chain.Epoch(100))
	runCalc(t, repo, adapter, chain.Epoch(100))

	require.Equal(t, 2, repo.saveCalls, "两次 Calc 各调一次 Save")
	require.Equal(t, 1, repo.len(), "同一 (epoch,address) 只应有一行")

	row := repo.get(100, "t01")
	require.NotNil(t, row)
	require.Equal(t, "100", row.Share.String())
	require.Equal(t, "5", row.Payable.String())
	require.Equal(t, "1", row.ClaimedPeriod.String())
	require.False(t, row.Tombstone)
}

// ctx.Empty() 直接返回：不请求节点、不写。
func TestCalcEmptyContextSkips(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 100, 0, 0))}, nil)}

	ctx, err := syncer.NewTestContextWithData(adapter, nil, chain.Epoch(100), true, nil)
	require.NoError(t, err)
	require.NoError(t, calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTask(repo).Calc(ctx))

	require.Empty(t, adapter.requested, "空高度不该请求节点")
	require.Zero(t, repo.saveCalls)
}

// ② nv29=false（本网未激活 NV29）不写。
func TestCalcNv29FalseWritesNothing(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{ledger: &londobell.RewardStreamLedger{Epoch: 100, Nv29: false, Denom: decimal.NewFromInt(1e18)}}

	runCalc(t, repo, adapter, chain.Epoch(100))

	require.Equal(t, []int64{100}, adapter.requested, "仍会请求节点（据此判断 nv29）")
	require.Zero(t, repo.saveCalls)
	require.Zero(t, repo.len())
}

// 取数出错时向上返回，不吞错、不写。
func TestCalcAdapterErrorPropagates(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{err: errors.New("boom")}

	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTask(repo)
	err := calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(100)))

	require.Error(t, err)
	require.Zero(t, repo.saveCalls)
}

// ③ 同一地址在两个高度各写一行、share 变化各写各的。
func TestCalcSameAddressTwoEpochsDifferentShare(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{}

	adapter.ledger = mkLedger(100, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 100, 0, 0))}, nil)
	runCalc(t, repo, adapter, chain.Epoch(100))

	adapter.ledger = mkLedger(200, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 250, 9, 2))}, nil)
	runCalc(t, repo, adapter, chain.Epoch(200))

	require.Equal(t, 2, repo.len(), "两个高度各留一行，互不覆盖")
	require.Equal(t, "100", repo.get(100, "t01").Share.String())
	require.Equal(t, "250", repo.get(200, "t01").Share.String())
	require.Equal(t, "9", repo.get(200, "t01").Payable.String())
	require.Equal(t, "2", repo.get(200, "t01").ClaimedPeriod.String())
}

// tombstone 遗留受益方：share 记 0；与活跃流里同地址时合并（tombstone=true）。
func TestCalcTombstoneShareZeroAndMerge(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 10, 5, 0))},
		[]*londobell.RewardStreamLedgerTombstone{mkTomb(9, mkTombRec("t01", 7), mkTombRec("t09", 3))},
	)}

	runCalc(t, repo, adapter, chain.Epoch(100))

	live := repo.get(100, "t01")
	require.Equal(t, "10", live.Share.String())
	require.Equal(t, "12", live.Payable.String(), "显式 5 + tombstone 7")
	require.True(t, live.Tombstone)

	gone := repo.get(100, "t09")
	require.NotNil(t, gone)
	require.True(t, gone.Share.IsZero(), "纯 tombstone 行 share 记 0")
	require.Equal(t, "3", gone.Payable.String())
	require.True(t, gone.Tombstone)
}

// 按地址合并（唯一键 (epoch,address)）：同地址跨两条显式流累加；隐式流不落行。
func TestCalcMergeAcrossStreamsAndSkipImplicit(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{
			mkStream(1, false, mkRec("t01", 4, 1, 1), mkRec("t02", 5, 0, 0)),
			mkStream(2, false, mkRec("t01", 6, 2, 3)),
			mkStream(3, true, mkRec("t99", 999, 999, 999)), // 隐式流：跳过
		}, nil)}

	runCalc(t, repo, adapter, chain.Epoch(100))

	require.Equal(t, 2, repo.len())
	t01 := repo.get(100, "t01")
	require.Equal(t, "10", t01.Share.String())
	require.Equal(t, "3", t01.Payable.String())
	require.Equal(t, "4", t01.ClaimedPeriod.String())
	require.Nil(t, repo.get(100, "t99"), "隐式流（矿工共识流）不落快照")
}

// 行 epoch 用同步器当前高度（请求节点时也传该高度）。
func TestCalcUsesSyncerEpoch(t *testing.T) {
	repo := newFakeRecipientRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(4110339,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 1, 0, 0))}, nil)}

	runCalc(t, repo, adapter, chain.Epoch(4110339))

	require.Equal(t, []int64{4110339}, adapter.requested)
	require.NotNil(t, repo.get(4110339, "t01"))
}

// ④ RollBack / HistoryClear 的 epoch 边界按 gte / lte 原样委托。
func TestCalcRollBackAndHistoryClearBounds(t *testing.T) {
	repo := newFakeRecipientRepo()
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTask(repo)

	require.NoError(t, calc.RollBack(context.Background(), chain.Epoch(500)))
	require.NoError(t, calc.HistoryClear(context.Background(), chain.Epoch(300)))

	require.Equal(t, []int64{500}, repo.gteArgs, "RollBack => delete where epoch >= gteEpoch")
	require.Equal(t, []int64{300}, repo.lteArgs, "HistoryClear => delete where epoch <= lteEpoch")
}

// ---- dal：用记录型假连接断言真正下发的 SQL（不拨号）----

func TestDalUpsertAndDeleteBounds(t *testing.T) {
	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)
	d := dal.NewRewardStreamRecipientDal(db)
	ctx := context.Background()

	// upsert：ON CONFLICT (epoch,address) DO UPDATE
	_ = d.SaveRewardStreamRecipients(ctx, []*po.RewardStreamRecipientEpoch{
		{Epoch: 1, Address: "f01", Share: decimal.NewFromInt(1)},
	})
	upsert := strings.ToLower(strings.Join(rec.Statements(), "\n"))
	require.Contains(t, upsert, "on conflict", "Save 必须是 upsert（ON CONFLICT）")
	require.Contains(t, upsert, "\"epoch\"", "冲突键包含 epoch")
	require.Contains(t, upsert, "\"address\"", "冲突键包含 address")
	require.Contains(t, upsert, "excluded", "命中冲突时覆盖已存在行")
	require.Contains(t, upsert, "tombstone", "覆盖包含 tombstone 列")

	// RollBack 方向：>=（且不得是 <=）
	rec.Reset()
	_ = d.DeleteRewardStreamRecipientsGteEpoch(ctx, chain.Epoch(100))
	del := strings.ToLower(rec.Statements()[0])
	require.Contains(t, del, "epoch >= ")
	require.NotContains(t, del, "epoch <= ")

	// HistoryClear 方向：<=（且不得是 >=）
	rec.Reset()
	_ = d.DeleteRewardStreamRecipientsLteEpoch(ctx, chain.Epoch(100))
	del = strings.ToLower(rec.Statements()[0])
	require.Contains(t, del, "epoch <= ")
	require.NotContains(t, del, "epoch >= ")

	// 区间查询：左闭右闭
	rec.Reset()
	_, _ = d.ListRewardStreamRecipientsByEpochRange(ctx, chain.NewLCRCRange(10, 20))
	q := strings.ToLower(rec.Statements()[0])
	require.Contains(t, q, "epoch >= ")
	require.Contains(t, q, "epoch <= ")
}
