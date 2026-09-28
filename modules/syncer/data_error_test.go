package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"

	logging "github.com/gozelle/logger"
	"github.com/gozelle/mix"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// fakeAgg 只实现跳过逻辑用到的聚合器方法（prepareSyncerEpoch 走 ParentTipset + Tipset）
type fakeAgg struct {
	londobell.Agg // 嵌入接口：未用到的上百个方法不实现
	parentErr     error
	tipsetErr     error
	parentCalls   int
	tipsetCalls   int
}

func (f *fakeAgg) ParentTipset(_ context.Context, start chain.Epoch) ([]*londobell.ParentTipset, error) {
	f.parentCalls++
	if f.parentErr != nil {
		return nil, f.parentErr
	}
	return []*londobell.ParentTipset{{Cids: []string{fmt.Sprintf("parent-of-%d", start)}}}, nil
}

func (f *fakeAgg) Tipset(_ context.Context, epoch chain.Epoch) ([]*londobell.Tipset, error) {
	f.tipsetCalls++
	if f.tipsetErr != nil {
		return nil, f.tipsetErr
	}
	return []*londobell.Tipset{{ID: epoch.Int64(), Cids: []string{fmt.Sprintf("cid-of-%d", epoch)}}}, nil
}

// fakeRepo 只实现跳过路径用到的 sync_syncer_epochs 写入与进度指针写入
type fakeRepo struct {
	repository.SyncerRepo // 嵌入接口：未用到的仓储方法不实现
	saved                 []*po.SyncSyncerEpoch
	err                   error

	syncers   []*po.SyncSyncer // SaveSyncer 写入的进度指针（chain.sync_syncers）
	syncerErr error
}

func (f *fakeRepo) SaveSyncSyncerEpoch(_ context.Context, item *po.SyncSyncerEpoch) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, item)
	return nil
}

func (f *fakeRepo) SaveSyncer(_ context.Context, task *po.SyncSyncer) error {
	if f.syncerErr != nil {
		return f.syncerErr
	}
	f.syncers = append(f.syncers, task)
	return nil
}

// fakeLedger 跳过台账的假实现（记录写入的台账行，并按 (syncer, epoch) 模拟幂等 upsert：
// 区间跳把「单高度行」升级为「区间行」就靠这个语义，
// 见 dal.SyncerDal.SaveSkippedEpoch）
type fakeLedger struct {
	items   []*po.SyncSkippedEpoch
	saveErr error
}

func (f *fakeLedger) SaveSkippedEpoch(_ context.Context, item *po.SyncSkippedEpoch) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	for _, v := range f.items {
		if v.Syncer == item.Syncer && v.Epoch == item.Epoch {
			// 保留首次登记的 created_at / first_failed_at，更新其余字段
			v.ErrorMessage = item.ErrorMessage
			v.Failures = item.Failures
			v.LastFailedAt = item.LastFailedAt
			v.ErrorClass = item.ErrorClass
			v.SkippedFrom = item.SkippedFrom
			v.SkippedTo = item.SkippedTo
			return nil
		}
	}
	f.items = append(f.items, item)
	return nil
}

func (f *fakeLedger) GetSkippedEpochs(_ context.Context, _ string, _ chain.LCRCRange) ([]*po.SyncSkippedEpoch, error) {
	return f.items, nil
}

const (
	testSkipThreshold = 5
	// 节点侧不可恢复错误的测试阈值（取小值，便于用例快速达阈值）
	testUnrecoverableThreshold = 3
	// 区间跳的测试门槛 / margin（与默认值同量级，便于按线上数字写用例）
	testStateGapMinGap = 1000
	testStateGapMargin = 200
)

func newSkipTestSyncer(threshold int64, agg londobell.Agg) (*Syncer, *fakeLedger, *fakeRepo) {
	ledger := &fakeLedger{}
	repo := &fakeRepo{}
	s := &Syncer{
		config: &config{
			name:                        "test-skip",
			dataErrorThreshold:          threshold,
			unrecoverableErrorThreshold: testUnrecoverableThreshold,
			stateGapJumpMinGap:          testStateGapMinGap,
			stateGapJumpMargin:          testStateGapMargin,
			skipLedger:                  ledger,
		},
		epoch:         chain.Epoch(100),
		log:           logging.NewLogger("test-skip"),
		failures:      newDataErrorTracker(threshold),
		unrecoverable: newDataErrorTracker(testUnrecoverableThreshold),
		repo:          repo,
	}
	s.agg = agg
	return s, ledger, repo
}

// 数据级错误样本：聚合器对某高度返回 code:1（链上第三方提交的 TerminateSectors 参数 bitfield 非最小编码，
// 聚合器内部 json.Marshal 失败）
func dataLevelAggError() error {
	return &londobell.BusinessError{
		Code:    londobell.CodeBusiness,
		Message: `json: error calling MarshalJSON for type *bitfield.BitField: invalid bitfield encoding`,
		URL:     "http://agg/aggregators/traces",
	}
}

// 节点侧「历史状态不可用」错误样本：与线上实测日志同文本（本地 lotus 只保留近期窗口，历史状态已被裁掉）
func plainStateTreeError() error {
	return errors.New("load state tree: failed to load state tree bafy2bzacea72i: failed to load hamt node: ipld: could not find bafy2bzaced")
}

// 同上，但被上游回透成业务码 code:1（本类必须优先于数据级判定，否则会被当成「这一格数据坏」逐高度爬）
func stateLevelAggError() error {
	return &londobell.BusinessError{
		Code:    londobell.CodeBusiness,
		Message: "load state tree: failed to load state tree bafy2bzacea72i: failed to load hamt node: blockstore get: not found",
		URL:     "http://agg/aggregators/state_tree",
	}
}

func TestClassifySyncError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"nil", nil, ErrorKindUnknown},
		{"聚合器业务码 code:1", dataLevelAggError(), ErrorKindData},
		{"业务错误被包裹", fmt.Errorf("exec epoch 100: %w", dataLevelAggError()), ErrorKindData},
		{"响应体解码失败", &londobell.DecodeError{Err: errors.New("invalid character"), URL: "http://agg/x"}, ErrorKindData},
		{"json 语法错误", &json.SyntaxError{}, ErrorKindData},
		{"json 类型错误", &json.UnmarshalTypeError{Value: "string"}, ErrorKindData},
		{"业务码里回透了网络错误", &londobell.BusinessError{
			Code: londobell.CodeBusiness, Message: "Post http://agg: dial tcp 10.0.0.1:80: connect: connection refused",
		}, ErrorKindTransport},
		{"连接被拒", &url.Error{Op: "Post", URL: "http://agg", Err: syscall.ECONNREFUSED}, ErrorKindTransport},
		{"net.OpError", &net.OpError{Op: "dial", Err: syscall.ECONNRESET}, ErrorKindTransport},
		{"EOF", fmt.Errorf("read response: %w", io.EOF), ErrorKindTransport},
		{"context 取消", fmt.Errorf("call agg: %w", context.Canceled), ErrorKindTransport},
		{"超时", context.DeadlineExceeded, ErrorKindTransport},
		{"纯文本连接错误", errors.New("dial tcp 127.0.0.1:3000: connect: connection refused"), ErrorKindTransport},
		{"软错误 mix.Warn", mix.Warnf("agg trace 未同步完毕"), ErrorKindNotReady},
		{"未获取到 miner 数据", errors.New("未获取到 miner 数据"), ErrorKindUnknown},
		{"数据库记录不存在", errors.New("record not found"), ErrorKindUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, ClassifySyncError(c.err), "错误分类不符: %v", c.err)
		})
	}
}

// ①数据级错误连续 N 次 → 断言被跳过且台账有记录
func TestDataErrorSkipAtThreshold(t *testing.T) {
	s, ledger, repo := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(1000)

	for i := 1; i < testSkipThreshold; i++ {
		skipped := s.recordEpochFailure(epoch, dataLevelAggError())
		require.False(t, skipped, "第 %d 次失败（阈值 %d）不应跳过", i, testSkipThreshold)
		require.Empty(t, ledger.items, "阈值以下不应登记台账")
		require.Empty(t, repo.saved, "阈值以下不应写 tipset 身份行")
	}

	// 第 N 次：达到阈值 ⇒ 登记并跳过
	skipped := s.recordEpochFailure(epoch, dataLevelAggError())
	require.True(t, skipped, "连续 %d 次数据级错误应跳过该高度", testSkipThreshold)

	rec, ok := s.skippedEpochFailure(epoch)
	require.True(t, ok, "该高度应处于「已跳过」状态")
	require.NotNil(t, rec.identity, "跳过应回填 tipset 身份行（保证链条连续）")

	// 台账有且只有一条记录，字段齐全
	require.Len(t, ledger.items, 1, "跳过必须登记台账，且同一高度只登一次")
	item := ledger.items[0]
	require.Equal(t, "test-skip", item.Syncer)
	require.Equal(t, epoch.Int64(), item.Epoch)
	require.Equal(t, int64(testSkipThreshold), item.Failures)
	require.Contains(t, item.ErrorMessage, "invalid bitfield encoding", "台账应保留错误摘要")
	require.False(t, item.FirstFailedAt.IsZero(), "首次失败时间应被记录")
	require.False(t, item.LastFailedAt.IsZero(), "末次失败时间应被记录")
	require.False(t, item.LastFailedAt.Before(item.FirstFailedAt), "末次失败时间不应早于首次失败时间")

	// 身份行已落库，且是该高度的 tipset 身份（keys/parent_keys 用于链条串联）
	require.Len(t, repo.saved, 1)
	require.Equal(t, epoch.Int64(), repo.saved[0].Epoch)
	require.Equal(t, []string{"cid-of-1000"}, []string(repo.saved[0].Keys))
	require.Equal(t, []string{"parent-of-1000"}, []string(repo.saved[0].ParentKeys))

	// 再失败若干次：仍然只有一条台账记录（不刷屏/不重复登记）
	for i := 0; i < testSkipThreshold+3; i++ {
		s.recordEpochFailure(epoch, dataLevelAggError())
	}
	require.Len(t, ledger.items, 1, "同一高度只登记一次台账")
}

// ②传输级错误连续 N+5 次 → 断言没有跳过
func TestTransportErrorNeverSkipped(t *testing.T) {
	s, ledger, repo := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(2000)

	transportErrors := []error{
		&url.Error{Op: "Post", URL: "http://agg", Err: syscall.ECONNREFUSED},
		fmt.Errorf("Post http://agg/aggregators/traces: %w", io.EOF),
		context.DeadlineExceeded,
		errors.New("dial tcp 10.0.0.1:80: connect: connection refused"),
		&net.OpError{Op: "read", Err: syscall.ECONNRESET},
	}
	for i := 0; i < testSkipThreshold+5; i++ {
		skipped := s.recordEpochFailure(epoch, transportErrors[i%len(transportErrors)])
		require.False(t, skipped, "传输级错误第 %d 次失败也绝不能跳过（应继续重试）", i+1)
	}

	_, ok := s.skippedEpochFailure(epoch)
	require.False(t, ok, "传输级错误不应使该高度进入「已跳过」状态")
	require.Empty(t, ledger.items, "传输级错误不应产生台账记录")
	require.Empty(t, repo.saved, "传输级错误不应写 tipset 身份行")
}

// ③阈值以下（N-1 次）不跳过
func TestDataErrorBelowThresholdNotSkipped(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(3000)

	for i := 0; i < testSkipThreshold-1; i++ {
		skipped := s.recordEpochFailure(epoch, dataLevelAggError())
		require.False(t, skipped)
	}

	_, ok := s.skippedEpochFailure(epoch)
	require.False(t, ok, "未达阈值不应跳过")
	require.Empty(t, ledger.items, "未达阈值不应登记台账")

	rec, exists := s.failures.get(epoch)
	require.True(t, exists)
	require.Equal(t, int64(testSkipThreshold-1), rec.failures, "应累计数据级失败次数")
}

// 传输级错误不打断数据级失败计数（聚合器抖动不应把计数清零，否则会再次出现静默停摆）
func TestTransportErrorKeepsDataErrorCount(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(3500)

	for i := 0; i < testSkipThreshold-1; i++ {
		s.recordEpochFailure(epoch, dataLevelAggError())
	}
	s.recordEpochFailure(epoch, errors.New("dial tcp 10.0.0.1:80: connect: connection refused"))
	skipped := s.recordEpochFailure(epoch, dataLevelAggError())

	require.True(t, skipped, "数据级失败累计到阈值应跳过（中间的传输级错误不清零计数）")
	require.Len(t, ledger.items, 1)
}

// 无法取到 tipset 身份时不跳过（跳过会造成链条空洞，一致性检查会误判分叉并触发回滚）
func TestSkipRequiresEpochIdentity(t *testing.T) {
	agg := &fakeAgg{parentErr: errors.New("dial tcp 10.0.0.1:80: connect: connection refused")}
	s, ledger, repo := newSkipTestSyncer(testSkipThreshold, agg)
	epoch := chain.Epoch(4000)

	var skipped bool
	for i := 0; i < testSkipThreshold+2; i++ {
		skipped = s.recordEpochFailure(epoch, dataLevelAggError())
	}
	require.False(t, skipped, "取不到 tipset 身份时不能跳过")
	_, ok := s.skippedEpochFailure(epoch)
	require.False(t, ok)
	require.Empty(t, ledger.items, "不能跳过就不应有台账记录")
	require.Empty(t, repo.saved)
}

// 台账写不进去时不跳过（不允许无留痕地跳过）
func TestSkipRequiresLedgerWrite(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	ledger.saveErr = errors.New("relation \"chain.sync_skipped_epochs\" does not exist")
	epoch := chain.Epoch(5000)

	var skipped bool
	for i := 0; i < testSkipThreshold+2; i++ {
		skipped = s.recordEpochFailure(epoch, dataLevelAggError())
	}
	require.False(t, skipped, "台账写入失败时不能跳过（必须保持原有重试语义）")
	_, ok := s.skippedEpochFailure(epoch)
	require.False(t, ok)
	require.Empty(t, ledger.items)
}

// 台账错误信息按长度截断（避免超长错误撑爆存储）
func TestSkipErrorMessageTruncated(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(1, &fakeAgg{})
	epoch := chain.Epoch(6000)

	long := make([]rune, maxDataErrorMessageLen+500)
	for i := range long {
		long[i] = 'x'
	}
	require.True(t, s.recordEpochFailure(epoch, &londobell.BusinessError{
		Code: londobell.CodeBusiness, Message: string(long), URL: "http://agg/aggregators/traces",
	}))
	require.Len(t, ledger.items, 1)
	require.LessOrEqual(t, len([]rune(ledger.items[0].ErrorMessage)), maxDataErrorMessageLen+len(" ...(truncated)"))
	require.Contains(t, ledger.items[0].ErrorMessage, "(truncated)")
}

// 默认阈值与配置覆盖
func TestDataErrorTrackerThreshold(t *testing.T) {
	require.Equal(t, int64(defaultDataErrorThreshold), newDataErrorTracker(0).threshold, "未配置时默认阈值应为 5")
	require.Equal(t, int64(defaultDataErrorThreshold), newDataErrorTracker(-1).threshold, "非法配置回退到默认阈值 5")
	require.Equal(t, int64(defaultDataErrorThreshold), newDataErrorTracker(5).threshold)
	require.Equal(t, int64(2), newDataErrorTracker(2).threshold)
}

// 计数与跳过状态在达到阈值后重置（回滚后重新判定）
func TestDataErrorTrackerReset(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(7000)

	for i := 0; i < testSkipThreshold; i++ {
		s.recordEpochFailure(epoch, dataLevelAggError())
	}
	require.Len(t, ledger.items, 1)

	s.failures.reset()
	_, ok := s.skippedEpochFailure(epoch)
	require.False(t, ok, "回滚后应重新累计（跳过状态清空）")

	// resetBelow 只清理更低高度
	s.failures.reach(epoch, dataLevelAggError())
	s.failures.resetBelow(epoch)
	_, exists := s.failures.get(epoch)
	require.True(t, exists, "resetBelow 不应清理该高度自身的记录")

	// 同一高度重新累计到阈值 → 再次登记（台账 upsert，进程内有新记录）
	ledger.items = nil
	var skipped bool
	for i := 0; i < testSkipThreshold; i++ {
		skipped = s.recordEpochFailure(epoch, dataLevelAggError())
	}
	require.True(t, skipped)
	require.Len(t, ledger.items, 1)
}

// =====================================================================================
// 防线②「节点侧状态不可用（不可恢复）」：新分类 + 独立阈值 + 一次跳过整段区间
// =====================================================================================

// 分类正判（5 条文本判据，含线上实测整行）与误判防护（网络/未就绪优先级更高，绝不误归本类）
func TestClassifyUnrecoverableStateError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorKind
	}{
		// --- 正判：节点侧状态不可用 ---
		{"实测整行", plainStateTreeError(), ErrorKindUnrecoverableState},
		{"failed to load state tree", errors.New("failed to load state tree bafy2bzacea72i: ipld: could not find"), ErrorKindUnrecoverableState},
		{"failed to load state", errors.New("failed to load state actor"), ErrorKindUnrecoverableState},
		{"ipld: could not find", errors.New("ipld: could not find bafy2bzaced"), ErrorKindUnrecoverableState},
		{"blockstore get", errors.New("blockstore get: not found"), ErrorKindUnrecoverableState},
		{"错误链里被包裹", fmt.Errorf("epoch 6357058 执行 ContextBuilder 错误: %w", plainStateTreeError()), ErrorKindUnrecoverableState},
		{"上游回透成业务码 code:1", stateLevelAggError(), ErrorKindUnrecoverableState},
		{"上游回透成解码失败", &londobell.DecodeError{Err: plainStateTreeError(), URL: "http://agg/x"}, ErrorKindUnrecoverableState},

		// --- 误判防护：网络错误里恰好带 ipld / blockstore / load state tree 字样 ⇒ 传输级 ---
		{"连接被拒文本带 ipld 字样", errors.New("dial tcp 172.31.38.30:1237: connect: connection refused: ipld: could not find"), ErrorKindTransport},
		{"超时文本带 blockstore get 字样", errors.New("read tcp 10.0.0.1:1237: i/o timeout: blockstore get failed"), ErrorKindTransport},
		{"net.OpError 文本带 load state tree", &net.OpError{Op: "read", Net: "tcp", Err: errors.New("load state tree: boom")}, ErrorKindTransport},
		{"url.Error 文本带 load state tree", &url.Error{Op: "Post", URL: "http://agg", Err: errors.New("load state tree: boom")}, ErrorKindTransport},
		{"EOF 包装带 ipld 字样", fmt.Errorf("read response: %w: ipld: could not find", io.EOF), ErrorKindTransport},
		{"context 超时带 ipld 字样", fmt.Errorf("ipld: could not find: %w", context.DeadlineExceeded), ErrorKindTransport},

		// --- 误判防护：未就绪/软错误（Warn / code:2）优先于本类 ---
		{"mix.Warn 文本带 load state tree", mix.Warnf("load state tree 未同步完毕"), ErrorKindNotReady},
		{"业务码 code:2 文本带 load state tree", &londobell.BusinessError{Code: londobell.CodeNotFound, Message: "load state tree: agg 还没索引到"}, ErrorKindNotReady},
		{"code:2 哨兵文本（impl.ErrNotFound）", errors.New("error not found"), ErrorKindUnknown},

		// --- 其它类别不受影响 ---
		{"业务码 code:1 无状态字样（数据级）", dataLevelAggError(), ErrorKindData},
		{"未知错误", errors.New("未获取到 miner 数据"), ErrorKindUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.want, ClassifySyncError(c.err), "错误分类不符: %v", c.err)
		})
	}
}

// 独立阈值：节点侧不可恢复错误连续达阈值（3）才登记，且只计入自己的计数器
func TestUnrecoverableErrorThresholdSkipsSingleEpoch(t *testing.T) {
	s, ledger, repo := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(1000)

	for i := 1; i < testUnrecoverableThreshold; i++ {
		require.False(t, s.recordEpochFailure(epoch, plainStateTreeError()),
			"第 %d 次（阈值 %d）不应跳过", i, testUnrecoverableThreshold)
		require.Empty(t, ledger.items, "阈值以下不应登记台账")
	}

	require.True(t, s.recordEpochFailure(epoch, plainStateTreeError()),
		"连续 %d 次节点侧不可恢复错误应跳过该高度", testUnrecoverableThreshold)

	require.Len(t, ledger.items, 1)
	item := ledger.items[0]
	require.Equal(t, "test-skip", item.Syncer)
	require.Equal(t, epoch.Int64(), item.Epoch)
	require.Equal(t, int64(testUnrecoverableThreshold), item.Failures, "应使用独立阈值（3），而不是数据级阈值（5）")
	require.NotNil(t, item.ErrorClass)
	require.Equal(t, errorClassUnrecoverableState, *item.ErrorClass)
	require.Nil(t, item.SkippedFrom, "单高度跳过不写区间列")
	require.Nil(t, item.SkippedTo, "单高度跳过不写区间列")
	require.Contains(t, item.ErrorMessage, "load state tree")

	// 跳过仍写 tipset 身份行（链条连续，这条保守前置不变）
	require.Len(t, repo.saved, 1)
	require.Equal(t, epoch.Int64(), repo.saved[0].Epoch)

	// 独立计数：数据级计数器没有被写入
	_, ok := s.failures.get(epoch)
	require.False(t, ok, "节点侧不可恢复错误不得计入数据级计数器")
	rec, exists := s.unrecoverable.get(epoch)
	require.True(t, exists)
	require.Equal(t, int64(testUnrecoverableThreshold), rec.failures)

	// 再失败不重复登记
	for i := 0; i < 3; i++ {
		s.recordEpochFailure(epoch, plainStateTreeError())
	}
	require.Len(t, ledger.items, 1)
}

// 区间跳的数学判据：目标 = 链头 − margin；min_gap 守卫；margin 大于 gap 不跳
func TestPlanStateGapJump(t *testing.T) {
	head := chain.Epoch(6_408_840)

	// 缺口 51,782（与线上实测同量级）⇒ 一次跳到 链头 − 200
	plan, ok := planStateGapJump(6_357_058, head, testStateGapMinGap, testStateGapMargin)
	require.True(t, ok)
	require.Equal(t, chain.Epoch(6_357_058), plan.From)
	require.Equal(t, chain.Epoch(6_408_840-200), plan.Next, "目标高度 = 链头 − margin")
	require.Equal(t, chain.Epoch(6_408_840-201), plan.Last(), "区间右端 = 新起点 − 1（含）")
	require.Equal(t, int64(6_408_840-200-6_357_058), plan.Count())

	// 缺口恰等于门槛 ⇒ 跳（边界）
	from := head - chain.Epoch(testStateGapMinGap)
	plan, ok = planStateGapJump(from, head, testStateGapMinGap, testStateGapMargin)
	require.True(t, ok, "缺口 == min_gap 应跳")
	require.Equal(t, head-chain.Epoch(testStateGapMargin), plan.Next)
	require.Equal(t, from, plan.From)

	// 缺口 < 门槛 ⇒ 不跳（只按原逻辑重试/单高度跳过）
	from = head - chain.Epoch(testStateGapMinGap) + 1
	_, ok = planStateGapJump(from, head, testStateGapMinGap, testStateGapMargin)
	require.False(t, ok, "缺口 %d < min_gap %d 不跳", head.Int64()-from.Int64(), testStateGapMinGap)

	// margin 大于缺口 ⇒ 不跳（否则只会原地退步）
	_, ok = planStateGapJump(head-500, head, 100, 600)
	require.False(t, ok, "margin 大于缺口时不跳")
	_, ok = planStateGapJump(head-500, head, 100, 500)
	require.False(t, ok, "margin == 缺口时不跳（目标不得等于起点）")

	// 链头未领先当前高度 ⇒ 不跳
	_, ok = planStateGapJump(head, head, testStateGapMinGap, testStateGapMargin)
	require.False(t, ok, "链头 == 当前高度不跳")
	_, ok = planStateGapJump(head+1, head, testStateGapMinGap, testStateGapMargin)
	require.False(t, ok, "链头落后于当前高度不跳")

	// 非法配置 ⇒ 不跳
	_, ok = planStateGapJump(100, head, 0, 200)
	require.False(t, ok, "min_gap <= 0 不跳")
	_, ok = planStateGapJump(100, head, -1, 200)
	require.False(t, ok, "min_gap < 0 不跳")
	_, ok = planStateGapJump(100, head, 1000, -1)
	require.False(t, ok, "margin < 0 不跳")
}

// 区间跳端到端（不含 DB）：达阈值先单高度登记 → 本批失败后判定区间跳 →
// 台账行升级为区间行、同步起点推进到 链头 − margin、进度指针写库
func TestStateGapJumpIntervalLedger(t *testing.T) {
	s, ledger, repo := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	from := chain.Epoch(6_357_058)
	head := chain.Epoch(6_408_840) // 缺口 51,782

	var skipped bool
	for i := 0; i < testUnrecoverableThreshold; i++ {
		skipped = s.recordEpochFailure(from, plainStateTreeError())
	}
	require.True(t, skipped)
	require.Equal(t, chain.Epoch(100), s.epoch, "达阈值时同步起点还没推进（区间跳由本批失败后判定）")
	require.Len(t, ledger.items, 1)
	require.Nil(t, ledger.items[0].SkippedFrom, "此时仍只是单高度记录")

	require.True(t, s.tryStateGapJump(head), "缺口远超门槛应完成区间跳")

	wantNext := head - chain.Epoch(testStateGapMargin)
	require.Equal(t, wantNext, s.epoch, "同步起点应一次推进到 链头 − margin")

	// 台账：同一 (syncer, epoch) 行被升级为区间行，覆盖整个区间
	require.Len(t, ledger.items, 1, "区间跳复用同一行（不新增重复行）")
	item := ledger.items[0]
	require.NotNil(t, item.SkippedFrom)
	require.NotNil(t, item.SkippedTo)
	require.Equal(t, from.Int64(), item.Epoch)
	require.Equal(t, from.Int64(), *item.SkippedFrom, "区间左端 = 被跳过的第一个高度")
	require.Equal(t, wantNext.Int64()-1, *item.SkippedTo, "区间右端（含）= 新起点 − 1")
	require.Equal(t, errorClassUnrecoverableState, *item.ErrorClass)
	require.Contains(t, item.ErrorMessage, "load state tree")
	require.Equal(t, int64(testUnrecoverableThreshold), item.Failures)

	// tipset 身份行（拿不到身份不跳的前置）+ 进度指针（重启不退回区间内）
	require.Len(t, repo.saved, 1)
	require.Equal(t, from.Int64(), repo.saved[0].Epoch)
	require.Len(t, repo.syncers, 1)
	require.Equal(t, "test-skip", repo.syncers[0].Name)
	require.Equal(t, wantNext.Int64()-1, repo.syncers[0].Epoch,
		"进度指针写为 新起点 − 1（Init 会以该值 +1 作为起点）")

	// 重复判定不会再次跳（新区间起点已高于被跳过区间，计数器里也没有新的达阈值高度）
	require.False(t, s.tryStateGapJump(head+5_000), "区间外没有达阈值高度时不应重复跳")
}

// 缺口不足门槛：不区间跳，保持「重试 / 单高度跳过」的原逻辑
func TestStateGapJumpRespectsMinGap(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	from := chain.Epoch(6_408_841)
	head := from + chain.Epoch(testStateGapMinGap) - 1 // 缺口 = 999 < 1000

	for i := 0; i < testUnrecoverableThreshold; i++ {
		s.recordEpochFailure(from, plainStateTreeError())
	}
	require.False(t, s.tryStateGapJump(head),
		"缺口 %d < 门槛 %d 时不许区间跳", head.Int64()-from.Int64(), testStateGapMinGap)
	require.Equal(t, chain.Epoch(100), s.epoch, "缺口不足时同步起点不动")
	require.Len(t, ledger.items, 1)
	require.Nil(t, ledger.items[0].SkippedFrom, "缺口不足时只保持单高度语义（不写区间列）")
}

// 台账写不进去时不许区间跳（也不许单高度跳过）
func TestStateGapJumpRequiresLedger(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	ledger.saveErr = errors.New(`relation "chain.sync_skipped_epochs" does not exist`)
	from := chain.Epoch(6_357_058)

	for i := 0; i < testUnrecoverableThreshold+2; i++ {
		require.False(t, s.recordEpochFailure(from, plainStateTreeError()))
	}
	require.False(t, s.tryStateGapJump(chain.Epoch(6_408_840)), "台账写失败时不许区间跳")
	require.Equal(t, chain.Epoch(100), s.epoch)
	require.Empty(t, ledger.items)
}

// 拿不到 tipset 身份时不许区间跳（跳过会造成无法解释的链条空洞）
func TestStateGapJumpRequiresIdentity(t *testing.T) {
	agg := &fakeAgg{parentErr: errors.New("dial tcp 10.0.0.1:80: connect: connection refused")}
	s, _, _ := newSkipTestSyncer(testSkipThreshold, agg)
	from := chain.Epoch(6_357_058)

	for i := 0; i < testUnrecoverableThreshold+2; i++ {
		require.False(t, s.recordEpochFailure(from, plainStateTreeError()))
	}
	require.False(t, s.tryStateGapJump(chain.Epoch(6_408_840)), "拿不到 tipset 身份时不许区间跳")
	require.Equal(t, chain.Epoch(100), s.epoch)
}

// 传输级错误：即使文本里恰好带 ipld / blockstore 字样，也绝不计入本类计数，且不清零已有计数
func TestTransportErrorNeverCountsUnrecoverable(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(1000)

	transportErrors := []error{
		&url.Error{Op: "Post", URL: "http://agg", Err: syscall.ECONNREFUSED},
		fmt.Errorf("Post http://agg/aggregators/state_tree: %w", io.EOF),
		context.DeadlineExceeded,
		// 网络错误里恰好带 ipld 字样：必须按传输级处理（不计入本类）
		errors.New("dial tcp 172.31.38.30:1237: connect: connection refused: ipld: could not find bafy2bzaced"),
	}
	for i := 0; i < testUnrecoverableThreshold+5; i++ {
		require.False(t, s.recordEpochFailure(epoch, transportErrors[i%len(transportErrors)]),
			"传输级错误第 %d 次也绝不能跳过", i+1)
	}
	require.Empty(t, ledger.items, "传输级错误不应产生台账记录")
	require.Empty(t, s.unrecoverable.items, "传输级错误既不计入也不清零（不应有任何计数记录）")

	// 2 次不可恢复 + 5 次传输级 + 1 次不可恢复 = 3 ⇒ 达阈值（传输级不清零）
	s.recordEpochFailure(epoch, plainStateTreeError())
	s.recordEpochFailure(epoch, plainStateTreeError())
	for i := 0; i < 5; i++ {
		s.recordEpochFailure(epoch, transportErrors[i%len(transportErrors)])
	}
	require.True(t, s.recordEpochFailure(epoch, plainStateTreeError()),
		"中间的传输级错误不应清零不可恢复错误计数")
	require.Len(t, ledger.items, 1)
	require.Equal(t, int64(testUnrecoverableThreshold), ledger.items[0].Failures)
}

// 数据级坏点不会触发区间跳（两类独立计数、独立阈值）
func TestDataErrorDoNotTriggerStateGapJump(t *testing.T) {
	s, ledger, _ := newSkipTestSyncer(testSkipThreshold, &fakeAgg{})
	epoch := chain.Epoch(6_357_058)

	for i := 0; i < testSkipThreshold+3; i++ {
		s.recordEpochFailure(epoch, dataLevelAggError())
	}
	require.Len(t, ledger.items, 1)
	require.NotNil(t, ledger.items[0].ErrorClass)
	require.Equal(t, errorClassData, *ledger.items[0].ErrorClass, "数据级错误登记为 data")
	require.Nil(t, ledger.items[0].SkippedFrom)

	require.False(t, s.tryStateGapJump(chain.Epoch(6_408_840)), "数据级坏点不触发区间跳")
	require.Equal(t, chain.Epoch(100), s.epoch)
	require.Empty(t, s.unrecoverable.items, "数据级错误不得计入不可恢复计数器")
}

// 一致性检查用：区间行按与查询区间求交展开（运维把指针改回区间起点重跑时，区间内高度视为「已知跳过」）
func TestCoveredEpochs(t *testing.T) {
	items := []*po.SyncSkippedEpoch{
		{Syncer: "test-skip", Epoch: 100}, // 单高度（老记录语义：skipped_from/to 为 NULL）
		{Syncer: "test-skip", Epoch: 200, SkippedFrom: ptrInt64(200), SkippedTo: ptrInt64(210)},
	}

	covered := coveredEpochs(items, chain.Epoch(95), chain.Epoch(205))
	require.Equal(t, map[int64]struct{}{
		100: {}, 200: {}, 201: {}, 202: {}, 203: {}, 204: {}, 205: {},
	}, covered, "单高度行只贡献 epoch；区间行只贡献与查询区间的交集（有界）")

	require.Empty(t, coveredEpochs(nil, chain.Epoch(1), chain.Epoch(10)))
	require.True(t, items[1].IsRange())
	require.False(t, items[0].IsRange())
}
