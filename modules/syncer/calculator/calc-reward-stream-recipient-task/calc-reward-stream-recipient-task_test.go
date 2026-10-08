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
// 快照表 upsert 的 ON CONFLICT，以及归集表 upsert 的合并（GREATEST/LEAST）与删除/查询的边界方向。

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

// ---- 假仓储：归集表 (address, period_start_epoch) 的**合并写**语义（模拟唯一键 + GREATEST/LEAST）----

type fakePeriodRepo struct {
	mu    sync.Mutex
	rows  map[string]*po.RewardStreamRecipientPeriod
	order []string

	upsertCalls int
	gteArgs     []int64
}

func newFakePeriodRepo() *fakePeriodRepo {
	return &fakePeriodRepo{rows: map[string]*po.RewardStreamRecipientPeriod{}}
}

func periodKey(addr string, periodStart int64) string { return fmt.Sprintf("%s|%d", addr, periodStart) }

func (r *fakePeriodRepo) UpsertRewardStreamRecipientPeriods(_ context.Context, items []*po.RewardStreamRecipientPeriod) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upsertCalls++
	for _, it := range items {
		k := periodKey(it.Address, it.PeriodStartEpoch)
		cur, ok := r.rows[k]
		if !ok {
			cp := *it
			r.rows[k] = &cp
			r.order = append(r.order, k)
			continue
		}
		// 合并语义：claimed 取 GREATEST、first 取 LEAST、last 取 GREATEST（幂等，不降值）。
		if it.ClaimedInPeriod.GreaterThan(cur.ClaimedInPeriod) {
			cur.ClaimedInPeriod = it.ClaimedInPeriod
		}
		if it.FirstEpoch < cur.FirstEpoch {
			cur.FirstEpoch = it.FirstEpoch
		}
		if it.LastEpoch > cur.LastEpoch {
			cur.LastEpoch = it.LastEpoch
		}
		// last_share：取「观测高度更大」那一行的值（与真实 dal 的 CASE WHEN excluded.last_epoch >= 已存 一致）。
		if it.LastEpoch >= cur.LastEpoch {
			cur.LastShare = it.LastShare
		}
	}
	return nil
}

func (r *fakePeriodRepo) ListRewardStreamRecipientPeriodsByAddresses(_ context.Context, addresses []string) ([]*po.RewardStreamRecipientPeriod, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*po.RewardStreamRecipientPeriod
	for _, k := range r.order {
		row := r.rows[k]
		for _, a := range addresses {
			if row.Address == a {
				out = append(out, row)
				break
			}
		}
	}
	return out, nil
}

func (r *fakePeriodRepo) DeleteRewardStreamRecipientPeriodsGteEpoch(_ context.Context, gteEpoch chain.Epoch) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gteArgs = append(r.gteArgs, gteEpoch.Int64())
	kept := make([]string, 0, len(r.order))
	for _, k := range r.order {
		if r.rows[k].LastEpoch >= gteEpoch.Int64() {
			delete(r.rows, k)
			continue
		}
		kept = append(kept, k)
	}
	r.order = kept
	return nil
}

func (r *fakePeriodRepo) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.rows)
}

func (r *fakePeriodRepo) get(addr string, periodStart int64) *po.RewardStreamRecipientPeriod {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rows[periodKey(addr, periodStart)]
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

// runCalc 用固定周期参数（nv29Epoch=0、periodLen=100）跑一个高度：
// 让既有快照用例的归集侧行为也确定可预测（不受生产常量 mainnet/calibnet 影响）。
func runCalc(t *testing.T, repo *fakeRecipientRepo, periodRepo *fakePeriodRepo, adapter *fakeLedgerAdapter, epoch chain.Epoch) {
	t.Helper()
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, 0, 100)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, epoch)))
}

// ============ 快照表（chain.reward_stream_recipient_epoch）既有用例 ============

// ① 同高度重跑不产生重复行（幂等 upsert）：同一 (epoch,address) 覆盖，不是追加。
func TestCalcIdempotentRerunSameEpoch(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 100, 5, 1))}, nil)}

	runCalc(t, repo, periodRepo, adapter, chain.Epoch(100))
	runCalc(t, repo, periodRepo, adapter, chain.Epoch(100))

	require.Equal(t, 2, repo.saveCalls, "两次 Calc 各调一次 Save")
	require.Equal(t, 1, repo.len(), "同一 (epoch,address) 只应有一行")

	row := repo.get(100, "t01")
	require.NotNil(t, row)
	require.Equal(t, "100", row.Share.String())
	require.Equal(t, "5", row.Payable.String())
	require.Equal(t, "1", row.ClaimedPeriod.String())
	require.False(t, row.Tombstone)
}

// ctx.Empty() 直接返回：不请求节点、不写（快照表与归集表都不写）。
func TestCalcEmptyContextSkips(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 100, 0, 0))}, nil)}

	ctx, err := syncer.NewTestContextWithData(adapter, nil, chain.Epoch(100), true, nil)
	require.NoError(t, err)
	require.NoError(t, calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTask(repo, periodRepo).Calc(ctx))

	require.Empty(t, adapter.requested, "空高度不该请求节点")
	require.Zero(t, repo.saveCalls)
	require.Zero(t, periodRepo.upsertCalls, "空高度不写归集表")
}

// ② nv29=false（本网未激活 NV29）不写。
func TestCalcNv29FalseWritesNothing(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{ledger: &londobell.RewardStreamLedger{Epoch: 100, Nv29: false, Denom: decimal.NewFromInt(1e18)}}

	runCalc(t, repo, periodRepo, adapter, chain.Epoch(100))

	require.Equal(t, []int64{100}, adapter.requested, "仍会请求节点（据此判断 nv29）")
	require.Zero(t, repo.saveCalls)
	require.Zero(t, periodRepo.upsertCalls, "nv29=false 不写归集表")
	require.Zero(t, repo.len())
}

// 取数出错时向上返回，不吞错、不写。
func TestCalcAdapterErrorPropagates(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{err: errors.New("boom")}

	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTask(repo, periodRepo)
	err := calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(100)))

	require.Error(t, err)
	require.Zero(t, repo.saveCalls)
	require.Zero(t, periodRepo.upsertCalls)
}

// ③ 同一地址在两个高度各写一行、share 变化各写各的。
func TestCalcSameAddressTwoEpochsDifferentShare(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{}

	adapter.ledger = mkLedger(100, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 100, 0, 0))}, nil)
	runCalc(t, repo, periodRepo, adapter, chain.Epoch(100))

	adapter.ledger = mkLedger(200, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 250, 9, 2))}, nil)
	runCalc(t, repo, periodRepo, adapter, chain.Epoch(200))

	require.Equal(t, 2, repo.len(), "两个高度各留一行，互不覆盖")
	require.Equal(t, "100", repo.get(100, "t01").Share.String())
	require.Equal(t, "250", repo.get(200, "t01").Share.String())
	require.Equal(t, "9", repo.get(200, "t01").Payable.String())
	require.Equal(t, "2", repo.get(200, "t01").ClaimedPeriod.String())
}

// tombstone 遗留受益方：share 记 0；与活跃流里同地址时合并（tombstone=true）。
// 同时断言：tombstone 遗留受益方**不进归集表**（claimed_period 记 0，不影响累计）。
func TestCalcTombstoneShareZeroAndMerge(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 10, 5, 0))},
		[]*londobell.RewardStreamLedgerTombstone{mkTomb(9, mkTombRec("t01", 7), mkTombRec("t09", 3))},
	)}

	runCalc(t, repo, periodRepo, adapter, chain.Epoch(100))

	live := repo.get(100, "t01")
	require.Equal(t, "10", live.Share.String())
	require.Equal(t, "12", live.Payable.String(), "显式 5 + tombstone 7")
	require.True(t, live.Tombstone)

	gone := repo.get(100, "t09")
	require.NotNil(t, gone)
	require.True(t, gone.Share.IsZero(), "纯 tombstone 行 share 记 0")
	require.Equal(t, "3", gone.Payable.String())
	require.True(t, gone.Tombstone)

	// 归集表只有显式流受益方 t01；t09 不出现在显式流里 ⇒ 不落行。
	// epoch=100、periodLen=100 ⇒ 周期起点 100。
	require.Equal(t, 1, periodRepo.len(), "只有显式流受益方进归集表")
	require.Nil(t, periodRepo.get("t09", 100), "纯 tombstone 受益方不进归集表")
	require.NotNil(t, periodRepo.get("t01", 100))
}

// 按地址合并（唯一键 (epoch,address)）：同地址跨两条显式流累加；隐式流不落行。
func TestCalcMergeAcrossStreamsAndSkipImplicit(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(100,
		[]*londobell.RewardStreamLedgerStream{
			mkStream(1, false, mkRec("t01", 4, 1, 1), mkRec("t02", 5, 0, 0)),
			mkStream(2, false, mkRec("t01", 6, 2, 3)),
			mkStream(3, true, mkRec("t99", 999, 999, 999)), // 隐式流：跳过
		}, nil)}

	runCalc(t, repo, periodRepo, adapter, chain.Epoch(100))

	require.Equal(t, 2, repo.len())
	t01 := repo.get(100, "t01")
	require.Equal(t, "10", t01.Share.String())
	require.Equal(t, "3", t01.Payable.String())
	require.Equal(t, "4", t01.ClaimedPeriod.String())
	require.Nil(t, repo.get(100, "t99"), "隐式流（矿工共识流）不落快照")

	// 归集表：同地址跨两条显式流 claimed 累加（1+3=4）；隐式流不归集。
	// epoch=100、periodLen=100 ⇒ 周期起点 100。
	require.Equal(t, 2, periodRepo.len())
	require.Equal(t, "4", periodRepo.get("t01", 100).ClaimedInPeriod.String(), "同地址跨流累加")
	require.Nil(t, periodRepo.get("t99", 100), "隐式流不归集")
}

// 行 epoch 用同步器当前高度（请求节点时也传该高度）。
func TestCalcUsesSyncerEpoch(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(4110339,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 1, 0, 0))}, nil)}

	runCalc(t, repo, periodRepo, adapter, chain.Epoch(4110339))

	require.Equal(t, []int64{4110339}, adapter.requested)
	require.NotNil(t, repo.get(4110339, "t01"))
}

// ④ RollBack / HistoryClear 的 epoch 边界按 gte / lte 原样委托（快照表 + 归集表）。
func TestCalcRollBackAndHistoryClearBounds(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTask(repo, periodRepo)

	require.NoError(t, calc.RollBack(context.Background(), chain.Epoch(500)))
	require.NoError(t, calc.HistoryClear(context.Background(), chain.Epoch(300)))

	require.Equal(t, []int64{500}, repo.gteArgs, "RollBack => 快照表 delete where epoch >= gteEpoch")
	require.Equal(t, []int64{500}, periodRepo.gteArgs, "RollBack => 归集表 delete where last_epoch >= gteEpoch")
	require.Equal(t, []int64{300}, repo.lteArgs, "HistoryClear => 快照表 delete where epoch <= lteEpoch")

	// HistoryClear 只清快照表：归集表**零删除**（本用例只断言「没调删除」，行为另有用例覆盖）。
	require.Len(t, periodRepo.gteArgs, 1, "HistoryClear 不触达归集表删除")
}

// ============ 归集表（chain.reward_stream_recipient_period）用例 ============

// ① 同一 (address, 周期) 跨多个高度取**最大值**（GREATEST），不是被最后一个高度覆盖。
// TestPeriodLastShareWriterNewestWins 锁住「离开前份额」的数据源：
// 归集行必须记录该周期内最近一次观测到的份额（新高度覆盖、旧高度回放不覆盖）。
func TestPeriodLastShareWriterNewestWins(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{}
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, 0, 1000)

	// 高度 10：份额 100 ⇒ 归集行 last_share = 100。
	adapter.ledger = mkLedger(10, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 100, 0, 0))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(10))))
	row := periodRepo.get("t01", 0)
	require.NotNil(t, row)
	require.Equal(t, "100", row.LastShare.String(), "归集行必须写入该高度的份额")

	// 高度 20：份额 250 ⇒ 更新的观测覆盖。
	adapter.ledger = mkLedger(20, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 250, 0, 0))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(20))))
	require.Equal(t, "250", periodRepo.get("t01", 0).LastShare.String(), "更新高度的份额覆盖旧值")

	// 高度 15（回放/重排）：份额 1 ⇒ 旧观测不得覆盖较新的值。
	adapter.ledger = mkLedger(15, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 1, 0, 0))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(15))))
	require.Equal(t, "250", periodRepo.get("t01", 0).LastShare.String(), "较旧高度的份额不得覆盖较新观测")

	// 多流同地址：同一高度按地址累加份额（口径同快照表 Share）。
	adapter.ledger = mkLedger(30, []*londobell.RewardStreamLedgerStream{
		mkStream(1, false, mkRec("t01", 30, 0, 0)),
		mkStream(2, false, mkRec("t01", 70, 0, 0)),
	}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(30))))
	require.Equal(t, "100", periodRepo.get("t01", 0).LastShare.String(), "同一高度多流按地址累加份额")
}

func TestPeriodSameAddressSamePeriodTakesMaxNotLast(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{}
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, 0, 1000)

	// 周期 [0,1000)：高度 10 报 claimed=7，高度 20 报 claimed=3（回放/重排）⇒ 取 7。
	adapter.ledger = mkLedger(10, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 7))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(10))))
	adapter.ledger = mkLedger(20, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 3))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(20))))

	require.Equal(t, 1, periodRepo.len(), "同一 (address,period) 只有一行")
	row := periodRepo.get("t01", 0)
	require.NotNil(t, row)
	require.Equal(t, "7", row.ClaimedInPeriod.String(), "GREATEST：较小值不覆盖较大值")
	require.Equal(t, int64(10), row.FirstEpoch, "first_epoch 取 LEAST")
	require.Equal(t, int64(20), row.LastEpoch, "last_epoch 取 GREATEST")
}

// ② 周期切换后新行、旧行不动。
func TestPeriodSwitchCreatesNewRowKeepsOld(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{}
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, 0, 1000)

	adapter.ledger = mkLedger(10, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 5))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(10))))

	// 高度 1000 是下一周期 [1000,2000) 的开端。
	adapter.ledger = mkLedger(1000, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 9))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(1000))))

	require.Equal(t, 2, periodRepo.len(), "跨周期各起一行")

	old := periodRepo.get("t01", 0)
	require.NotNil(t, old)
	require.Equal(t, "5", old.ClaimedInPeriod.String(), "旧周期行不被新周期覆盖")
	require.Equal(t, int64(10), old.FirstEpoch)
	require.Equal(t, int64(10), old.LastEpoch)

	nw := periodRepo.get("t01", 1000)
	require.NotNil(t, nw)
	require.Equal(t, "9", nw.ClaimedInPeriod.String())
}

// ③ 重跑同高度幂等（值不变、行数不变）。
func TestPeriodRerunSameEpochIdempotent(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{ledger: mkLedger(10,
		[]*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 4))}, nil)}
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, 0, 1000)

	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(10))))
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(10))))

	require.Equal(t, 2, periodRepo.upsertCalls, "两次 Calc 各调一次 Upsert")
	require.Equal(t, 1, periodRepo.len(), "同一 (address,period) 只应有一行")
	row := periodRepo.get("t01", 0)
	require.Equal(t, "4", row.ClaimedInPeriod.String())
	require.Equal(t, int64(10), row.FirstEpoch)
	require.Equal(t, int64(10), row.LastEpoch)
}

// ④ RollBack 边界（>= 方向）：只删 last_epoch >= gteEpoch 的行。
func TestPeriodRollBackDeletesLastEpochGte(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{}
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, 0, 1000)

	adapter.ledger = mkLedger(10, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 1))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(10))))
	adapter.ledger = mkLedger(1000, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 2))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(1000))))
	adapter.ledger = mkLedger(2000, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 3))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(2000))))
	require.Equal(t, 3, periodRepo.len())

	require.NoError(t, calc.RollBack(context.Background(), chain.Epoch(1000)))

	require.Equal(t, []int64{1000}, periodRepo.gteArgs, "边界按 gteEpoch 原样委托")
	require.Equal(t, 1, periodRepo.len(), "last_epoch>=1000 的两行被删，last_epoch=10 的保留")
	require.NotNil(t, periodRepo.get("t01", 0), "last_epoch=10 < 1000 保留")
	require.Nil(t, periodRepo.get("t01", 1000), "last_epoch=1000 删除（>= 含等于）")
	require.Nil(t, periodRepo.get("t01", 2000), "last_epoch=2000 删除")
}

// ⑤ HistoryClear 不删任何归集行（累计必须跨周期保留）。
func TestPeriodHistoryClearKeepsAllRows(t *testing.T) {
	repo := newFakeRecipientRepo()
	periodRepo := newFakePeriodRepo()
	adapter := &fakeLedgerAdapter{}
	calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, 0, 1000)

	adapter.ledger = mkLedger(10, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 1))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(10))))
	adapter.ledger = mkLedger(2000, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 3))}, nil)
	require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(2000))))
	require.Equal(t, 2, periodRepo.len())

	require.NoError(t, calc.HistoryClear(context.Background(), chain.Epoch(3000)))

	require.Equal(t, 2, periodRepo.len(), "HistoryClear 不清理归集表：0 行被删")
	require.NotNil(t, periodRepo.get("t01", 0))
	require.NotNil(t, periodRepo.get("t01", 2000))
	require.Empty(t, periodRepo.gteArgs, "HistoryClear 不触达归集表删除")

	// 快照表仍按 lte 清理（方向不变）。
	require.Equal(t, []int64{3000}, repo.lteArgs)
}

// ⑥ periodStart 计算：用主网 262974 与 calibnet 2880 两个 periodLen 各验周期首/周期末边界。
func TestPeriodStartBoundariesForTwoNetworks(t *testing.T) {
	cases := []struct {
		name      string
		nv29Epoch int64
		periodLen int64
	}{
		{"mainnet-periodLen-262974", 1000, 262974},
		{"calibnet-periodLen-2880", 4109133, 2880},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRecipientRepo()
			periodRepo := newFakePeriodRepo()
			adapter := &fakeLedgerAdapter{}
			calc := calc_reward_stream_recipient_task.NewCalcRewardStreamRecipientTaskWithPeriod(repo, periodRepo, tc.nv29Epoch, tc.periodLen)

			periodFirst := tc.nv29Epoch                   // 周期首高度
			periodLast := tc.nv29Epoch + tc.periodLen - 1 // 周期末高度（仍属同一周期）
			nextFirst := tc.nv29Epoch + tc.periodLen      // 下一周期首高度

			adapter.ledger = mkLedger(periodFirst, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 1))}, nil)
			require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(periodFirst))))
			adapter.ledger = mkLedger(periodLast, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 2))}, nil)
			require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(periodLast))))
			adapter.ledger = mkLedger(nextFirst, []*londobell.RewardStreamLedgerStream{mkStream(1, false, mkRec("t01", 0, 0, 3))}, nil)
			require.NoError(t, calc.Calc(syncer.NewTestContext(adapter, nil, chain.Epoch(nextFirst))))

			require.Equal(t, 2, periodRepo.len(), "周期首/末合一行，下一周期首另起一行")

			first := periodRepo.get("t01", periodFirst)
			require.NotNil(t, first, "周期末归入本周期起点 %d", periodFirst)
			require.Equal(t, int64(periodFirst), first.FirstEpoch)
			require.Equal(t, int64(periodLast), first.LastEpoch)
			require.Equal(t, "2", first.ClaimedInPeriod.String(), "周期内取最大")

			next := periodRepo.get("t01", nextFirst)
			require.NotNil(t, next)
			require.Equal(t, "3", next.ClaimedInPeriod.String())
		})
	}
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

// 归集表 dal：断言真正下发的 upsert（合并语义 GREATEST/LEAST + 冲突键）与删除/查询边界。
func TestPeriodDalUpsertMergeAndDeleteBounds(t *testing.T) {
	rec := &offlinereplay.Recorder{}
	db := offlinereplay.OpenTestDB(t, rec)
	d := dal.NewRewardStreamRecipientPeriodDal(db)
	ctx := context.Background()

	// upsert：ON CONFLICT (address, period_start_epoch) DO UPDATE，合并取 GREATEST / LEAST。
	_ = d.UpsertRewardStreamRecipientPeriods(ctx, []*po.RewardStreamRecipientPeriod{
		{Address: "f01", PeriodStartEpoch: 100, ClaimedInPeriod: decimal.NewFromInt(1), FirstEpoch: 100, LastEpoch: 100},
	})
	upsert := strings.ToLower(strings.Join(rec.Statements(), "\n"))
	require.Contains(t, upsert, "on conflict", "Upsert 必须是 upsert（ON CONFLICT）")
	require.Contains(t, upsert, "\"address\"", "冲突键包含 address")
	require.Contains(t, upsert, "\"period_start_epoch\"", "冲突键包含 period_start_epoch")
	require.Contains(t, upsert, "greatest", "claimed_in_period 合并取 GREATEST")
	require.Contains(t, upsert, "least", "first_epoch 合并取 LEAST")
	require.Contains(t, upsert, "excluded", "命中冲突时引用新值")
	// last_share 合并：只在「新观测不比已存更旧」时替换（离场行的「离开前份额」靠它）。
	require.Contains(t, upsert, "last_share", "SQL 含 last_share 列")
	require.Contains(t, upsert, "case when", "last_share 用 CASE 决定取哪一行的值")
	require.Contains(t, upsert, "excluded.last_share", "last_share 命中冲突时引用新值")
	require.Contains(t, upsert, "period_start_epoch", "SQL 含周期起点列")

	// RollBack 方向：delete where last_epoch >= gteEpoch（且不得是 <=）。
	rec.Reset()
	_ = d.DeleteRewardStreamRecipientPeriodsGteEpoch(ctx, chain.Epoch(100))
	del := strings.ToLower(rec.Statements()[0])
	require.Contains(t, del, "last_epoch >= ")
	require.NotContains(t, del, "last_epoch <= ")

	// 按地址批量取：in 查询 + 定序。
	rec.Reset()
	_, _ = d.ListRewardStreamRecipientPeriodsByAddresses(ctx, []string{"f01", "f02"})
	q := strings.ToLower(rec.Statements()[0])
	require.Contains(t, q, "in ")
	require.Contains(t, q, "period_start_epoch asc")

	// 空地址切片：不发 SQL。
	rec.Reset()
	items, err := d.ListRewardStreamRecipientPeriodsByAddresses(ctx, nil)
	require.NoError(t, err)
	require.Nil(t, items)
	require.Empty(t, rec.Statements(), "空地址切片不应下发 SQL")
}
