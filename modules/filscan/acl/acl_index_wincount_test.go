package acl

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	logging "github.com/gozelle/logger"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 以下三个 fake 都用「嵌入接口 + 只实现被调用的方法」的写法：其余方法不会被调用
// （调用即 nil panic，属测试本身写错）。这样不必为 9 个方法的 AggIndexAcl 手写桩。

type fakeIndexAgg struct {
	AggIndexAcl

	streams []*londobell.RewardStream
	err     error

	calls int
	start chain.Epoch
	end   chain.Epoch
}

func (f *fakeIndexAgg) RewardStreams(_ context.Context, start, end chain.Epoch) ([]*londobell.RewardStream, error) {
	f.calls++
	f.start, f.end = start, end
	return f.streams, f.err
}

type fakeIndexAdapter struct {
	AdapterIndexAcl

	state map[string]interface{}
	err   error

	calls int
}

func (f *fakeIndexAdapter) Actor(_ context.Context, _ chain.SmartAddress, _ *chain.Epoch) (*londobell.ActorState, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return &londobell.ActorState{State: f.state}, nil
}

type fakeWinCountRepo struct {
	sum     int64
	covered int64
	err     error

	calls int
	start chain.Epoch
	end   chain.Epoch
}

func (f *fakeWinCountRepo) GetWinCountRewardStats(_ context.Context, start, end chain.Epoch) (int64, int64, error) {
	f.calls++
	f.start, f.end = start, end
	return f.sum, f.covered, f.err
}

// captureAclLogs 把 acl 包日志抓到内存，供「兜底必须打 Warn」断言（同 browser 包的做法）。
func captureAclLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	logging.SetPrimaryCore(zapcore.NewCore(
		zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(buf),
		zapcore.DebugLevel,
	))
	logging.SetAllLoggers(logging.LevelDebug)
	t.Cleanup(func() { logging.SetupLogging(logging.GetConfig()) })
	return buf
}

// 旧口径兜底值：ThisEpochReward / 5。
const legacyThisEpochReward = "100000000000000000000" // 100 FIL

func newTestAcl(agg *fakeIndexAgg, adapter *fakeIndexAdapter, repo *fakeWinCountRepo) *IndexAclImpl {
	return NewIndexAclImpl(agg, adapter, repo)
}

// twoRealStreams 两条相隔 2880（一整天）的真实形态整点快照：v18 口径用 TotalStoragePowerReward。
// 跨度必须是整 24 小时——rewardStreamTailBounds 会按「末行−2880」归一化端点，
// 跨度不足时拿不到左锚点、只能退回首行（那是「数据太短」的兜底，不是正常路径）。
func twoRealStreams() []*londobell.RewardStream {
	return []*londobell.RewardStream{
		{Epoch: 6426960, TotalStoragePowerReward: decimal.RequireFromString("100000000000000000000000")},
		{Epoch: 6429840, TotalStoragePowerReward: decimal.RequireFromString("165000000000000000000000")},
	}
}

// 正常路径：Δ矿工实收 / Δ赢票数，且窗口为 [T-2880, T+1)。
func TestGetWinCountRewardMeasuredSuccess(t *testing.T) {
	agg := &fakeIndexAgg{streams: twoRealStreams()}
	adapter := &fakeIndexAdapter{err: errors.New("正常路径不该取旧口径 actor")}
	repo := &fakeWinCountRepo{sum: 100, covered: 2880}
	a := newTestAcl(agg, adapter, repo)

	const epoch chain.Epoch = 6429840
	got, err := a.GetWinCountReward(context.Background(), epoch)
	if err != nil {
		t.Fatalf("不应报错: %s", err)
	}
	// 6.5e22 atto / 100 = 6.5e20 atto。
	want := decimal.RequireFromString("650000000000000000000")
	if !got.Equal(want) {
		t.Fatalf("实测每赢票奖励错: got %s want %s", got, want)
	}
	if agg.start != epoch-2880-720 || agg.end != epoch+1 {
		t.Fatalf("取数窗口错: got [%d,%d) want [%d,%d)", agg.start, agg.end, epoch-2880-720, epoch+1)
	}
	// 分母与分子同窗：端点＝归一化后的 [末行−2880, 末行]（本例链头恰在整点，故等于请求窗口末端）。
	if repo.start != 6426960 || repo.end != 6429840 {
		t.Fatalf("赢票数窗口错: got [%d,%d) want [6426960,6429840)", repo.start, repo.end)
	}
	if adapter.calls != 0 {
		t.Fatalf("正常路径不该回退旧口径（调用了 actor %d 次）", adapter.calls)
	}
}

// 兜底 1：分母为 0 → 返回 0（不 panic、不回退、不碰 actor）。
func TestGetWinCountRewardZeroDenominator(t *testing.T) {
	agg := &fakeIndexAgg{streams: twoRealStreams()}
	adapter := &fakeIndexAdapter{err: errors.New("分母 0 不该回退旧口径")}
	repo := &fakeWinCountRepo{sum: 0, covered: 0}
	a := newTestAcl(agg, adapter, repo)

	got, err := a.GetWinCountReward(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("分母为 0 不应报错: %s", err)
	}
	if !got.IsZero() {
		t.Fatalf("分母为 0 应返回 0，得到 %s", got)
	}
	if adapter.calls != 0 {
		t.Fatalf("分母为 0 不该回退旧口径（调用了 actor %d 次）", adapter.calls)
	}
}

// 兜底 2：两时点计数器缺失（快照不足两行）→ 旧口径 + WARN。
func TestGetWinCountRewardInsufficientSnapshotsFallsBack(t *testing.T) {
	logs := captureAclLogs(t)
	agg := &fakeIndexAgg{streams: twoRealStreams()[:1]}
	adapter := &fakeIndexAdapter{state: map[string]interface{}{"ThisEpochReward": legacyThisEpochReward}}
	repo := &fakeWinCountRepo{sum: 1000, covered: 2880}
	a := newTestAcl(agg, adapter, repo)

	got, err := a.GetWinCountReward(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("回退路径不应报错: %s", err)
	}
	if !got.Equal(decimal.RequireFromString("20000000000000000000")) { // 100 FIL / 5
		t.Fatalf("应回退旧口径 20 FIL，得到 %s", got)
	}
	if !strings.Contains(logs.String(), "reward_streams") || !strings.Contains(logs.String(), "回退旧口径") {
		t.Fatalf("必须打 WARN 说明回退，实际日志:\n%s", logs.String())
	}
}

// 兜底 3：reward_streams 接口失败 → 旧口径 + WARN。
func TestGetWinCountRewardStreamsErrorFallsBack(t *testing.T) {
	logs := captureAclLogs(t)
	agg := &fakeIndexAgg{err: errors.New("dial tcp: connection refused")}
	adapter := &fakeIndexAdapter{state: map[string]interface{}{"ThisEpochReward": legacyThisEpochReward}}
	repo := &fakeWinCountRepo{sum: 1000, covered: 2880}
	a := newTestAcl(agg, adapter, repo)

	got, err := a.GetWinCountReward(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("回退路径不应报错: %s", err)
	}
	if !got.Equal(decimal.RequireFromString("20000000000000000000")) {
		t.Fatalf("应回退旧口径 20 FIL，得到 %s", got)
	}
	if !strings.Contains(logs.String(), "回退旧口径") {
		t.Fatalf("接口失败必须打 WARN，实际日志:\n%s", logs.String())
	}
}

// 兜底 4：窗口高度覆盖率 < 90% → 旧口径 + WARN。
func TestGetWinCountRewardLowCoverageFallsBack(t *testing.T) {
	logs := captureAclLogs(t)
	agg := &fakeIndexAgg{streams: twoRealStreams()}
	adapter := &fakeIndexAdapter{state: map[string]interface{}{"ThisEpochReward": legacyThisEpochReward}}
	repo := &fakeWinCountRepo{sum: 120, covered: 1000} // 1000/2880 ≈ 34.7%
	a := newTestAcl(agg, adapter, repo)

	got, err := a.GetWinCountReward(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("回退路径不应报错: %s", err)
	}
	if !got.Equal(decimal.RequireFromString("20000000000000000000")) {
		t.Fatalf("覆盖不足应回退旧口径 20 FIL，得到 %s", got)
	}
	if !strings.Contains(logs.String(), "覆盖不足") {
		t.Fatalf("覆盖不足必须打 WARN，实际日志:\n%s", logs.String())
	}
}

// 兜底 5：读 PG 赢票数失败 → 旧口径 + WARN。
func TestGetWinCountRewardWinCountReadErrorFallsBack(t *testing.T) {
	logs := captureAclLogs(t)
	agg := &fakeIndexAgg{streams: twoRealStreams()}
	adapter := &fakeIndexAdapter{state: map[string]interface{}{"ThisEpochReward": legacyThisEpochReward}}
	repo := &fakeWinCountRepo{err: errors.New("pg: connection reset")}
	a := newTestAcl(agg, adapter, repo)

	got, err := a.GetWinCountReward(context.Background(), 6429840)
	if err != nil {
		t.Fatalf("回退路径不应报错: %s", err)
	}
	if !got.Equal(decimal.RequireFromString("20000000000000000000")) {
		t.Fatalf("读 PG 失败应回退旧口径 20 FIL，得到 %s", got)
	}
	if !strings.Contains(logs.String(), "chain.miner_win_counts") {
		t.Fatalf("读 PG 失败必须打 WARN，实际日志:\n%s", logs.String())
	}
}

// 旧口径也失败：返回错误（由 biz 层记日志并给 0，首页不得 500），且不 panic。
func TestGetWinCountRewardLegacyAlsoFailsReturnsError(t *testing.T) {
	agg := &fakeIndexAgg{err: errors.New("agg down")}
	adapter := &fakeIndexAdapter{err: errors.New("adapter down")}
	repo := &fakeWinCountRepo{sum: 1, covered: 2880}
	a := newTestAcl(agg, adapter, repo)

	_, err := a.GetWinCountReward(context.Background(), 6429840)
	if err == nil {
		t.Fatal("旧口径也失败时应返回错误（上游仅记日志，不会 500）")
	}
}

// minerMintedDelta 纯函数：nil/单行/首尾同高/乱序。
func TestMinerMintedDelta(t *testing.T) {
	if _, ok := minerMintedDelta(nil); ok {
		t.Fatal("nil 序列应 ok=false")
	}
	if _, ok := minerMintedDelta([]*londobell.RewardStream{{Epoch: 1}}); ok {
		t.Fatal("单行应 ok=false")
	}
	if _, ok := minerMintedDelta([]*londobell.RewardStream{nil, nil}); ok {
		t.Fatal("全 nil 应 ok=false")
	}
	if _, ok := minerMintedDelta([]*londobell.RewardStream{{Epoch: 5}, {Epoch: 5}}); ok {
		t.Fatal("首尾同高应 ok=false")
	}

	// 乱序也必须取到真正的最小/最大。
	lo := &londobell.RewardStream{Epoch: 1000, TotalStoragePowerReward: decimal.RequireFromString("100000000000000000000000")}
	hi := &londobell.RewardStream{Epoch: 1120, TotalStoragePowerReward: decimal.RequireFromString("165000000000000000000000")}
	delta, ok := minerMintedDelta([]*londobell.RewardStream{hi, lo})
	if !ok {
		t.Fatal("两行应 ok=true")
	}
	if !delta.Equal(decimal.RequireFromString("65000000000000000000000")) {
		t.Fatalf("差分错: %s", delta)
	}
}

// 端点归一化：整点快照跨度 3000 epoch 时，差分必须只取 [末行−2880, 末行]，不能把首行当左端。
//
// 这是「24h 三流少算 1 小时（低约 4%）」的根因：聚合器最新一行可能是链头整点、也可能是前一个小时，
// 直接用首行/末行求差，跨度恒在 2640~2880 之间漂（实测主网 24h 档只有 23 个点）。
func TestRewardStreamTailBoundsExact24h(t *testing.T) {
	row := func(e chain.Epoch, v int64) *londobell.RewardStream {
		return &londobell.RewardStream{Epoch: int64(e), TotalStoragePowerReward: decimal.NewFromInt(v)}
	}
	const last chain.Epoch = 6429840
	streams := []*londobell.RewardStream{
		row(last-3000, 1000), // 多取的那一小时（首行，不应作为左端）
		row(last-2880, 2000), // ← 正确左端
		row(last-120, 3000),
		row(last, 4000),
	}
	start, end, ok := rewardStreamTailBounds(streams)
	if !ok {
		t.Fatal("应 ok=true")
	}
	if start != last-2880 || end != last {
		t.Fatalf("端点错: got [%d,%d] want [%d,%d]", start, end, last-2880, last)
	}
	d := rewardStreamDeltas24H(streams)
	if !d.OK {
		t.Fatal("差分应 OK")
	}
	if !d.Miner.Equal(decimal.NewFromInt(2000)) {
		t.Fatalf("差分应取 [末行−2880, 末行]（4000−2000=2000），得到 %s", d.Miner)
	}
}

// 序列短于 24h（快照太稀）⇒ 退回序列首行，保持既有「至少两行即可用」的行为。
func TestRewardStreamTailBoundsShortSeriesFallsBack(t *testing.T) {
	streams := []*londobell.RewardStream{
		{Epoch: 1000, TotalStoragePowerReward: decimal.NewFromInt(1000)},
		{Epoch: 1120, TotalStoragePowerReward: decimal.NewFromInt(2000)},
	}
	start, end, ok := rewardStreamTailBounds(streams)
	if !ok {
		t.Fatal("两行应 ok=true")
	}
	if start != 1000 || end != 1120 {
		t.Fatalf("应退回首行: got [%d,%d]", start, end)
	}
}

// 非整点链头：取数起点要 floor 到整点再往前 1 小时（保证「末行−2880」那行在结果里）。
func TestRewardStreamsFetchStartAligned(t *testing.T) {
	// 非整点链头：floor(T−2880) 再往前 6 小时（余量要能吃掉快照落后链头的数小时）。
	if got, want := rewardStreamsFetchStart(6429887), chain.Epoch(6426240); got != want {
		t.Fatalf("取数起点错: got %d want %d", got, want)
	}
	// 整点链头同理。
	if got, want := rewardStreamsFetchStart(6429840), chain.Epoch(6426240); got != want {
		t.Fatalf("整点链头取数起点错: got %d want %d", got, want)
	}
}
