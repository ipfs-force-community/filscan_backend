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

// fakeRepo 只实现跳过路径用到的 sync_syncer_epochs 写入
type fakeRepo struct {
	repository.SyncerRepo // 嵌入接口：未用到的仓储方法不实现
	saved                 []*po.SyncSyncerEpoch
	err                   error
}

func (f *fakeRepo) SaveSyncSyncerEpoch(_ context.Context, item *po.SyncSyncerEpoch) error {
	if f.err != nil {
		return f.err
	}
	f.saved = append(f.saved, item)
	return nil
}

// fakeLedger 跳过台账的假实现（记录写入的台账行）
type fakeLedger struct {
	items   []*po.SyncSkippedEpoch
	saveErr error
}

func (f *fakeLedger) SaveSkippedEpoch(_ context.Context, item *po.SyncSkippedEpoch) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.items = append(f.items, item)
	return nil
}

func (f *fakeLedger) GetSkippedEpochs(_ context.Context, _ string, _ chain.LCRCRange) ([]*po.SyncSkippedEpoch, error) {
	return f.items, nil
}

const testSkipThreshold = 5

func newSkipTestSyncer(threshold int64, agg londobell.Agg) (*Syncer, *fakeLedger, *fakeRepo) {
	ledger := &fakeLedger{}
	repo := &fakeRepo{}
	s := &Syncer{
		config: &config{
			name:               "test-skip",
			dataErrorThreshold: threshold,
			skipLedger:         ledger,
		},
		epoch:    chain.Epoch(100),
		log:      logging.NewLogger("test-skip"),
		failures: newDataErrorTracker(threshold),
		repo:     repo,
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
