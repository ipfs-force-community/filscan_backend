package syncer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gozelle/mix"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

// 本文件实现「数据级错误连续 N 次失败 ⇒ 跳过该高度并登记，传输级错误照旧重试」。
//
// 背景：聚合器 /aggregators/traces 对某个高度返回业务码 code:1（链上第三方提交的
// TerminateSectors 参数里 bitfield 非最小编码，读取时 json.Marshal 失败）时，原逻辑
// 把该错误当成「非回滚类错误」，在 modules/syncer/syncer.go 的重试路径上**同一高度
// 无限重试、不跳过、不告警**，15 秒一轮跑了 18 天。这里补上「有上限的重试 + 留痕跳过」。

const (
	// defaultDataErrorThreshold 数据级错误连续失败次数的默认阈值：
	// 同一高度连续 5 轮都是数据级失败 ⇒ 登记并跳过该高度。
	// 配置项：syncer.data_error_threshold（见 modules/common/config/config.go）
	defaultDataErrorThreshold int64 = 5

	// maxDataErrorMessageLen 台账中错误信息的最大长度（按字符截断，防止超长错误撑爆存储）
	maxDataErrorMessageLen = 1000
)

// ErrorKind 同步器错误的分类，决定「该高度能否被跳过」。
//
// 判据（改分类前先读这里，判据优先级：类型 > 错误码 > 文本兜底）：
//
//	ErrorKindData（数据级，可计数、达阈值可跳过）：
//	  1. *londobell.BusinessError：聚合器/适配器 HTTP 200 但响应体 code != 0
//	     （例如 code:1）。同一高度重试不会自愈 —— 服务端明确处理了这次请求并失败。
//	     例外：若其 Message 命中传输级文本兜底（服务端把下游网络错误回透成业务码），
//	     一律按传输级处理，避免误跳。
//	  2. *londobell.DecodeError：响应体存在但 json 解码失败。
//	  3. encoding/json 的 *json.SyntaxError / *json.UnmarshalTypeError（上游未包装时）。
//
//	ErrorKindTransport（传输级，绝不计入跳过、照旧重试）：
//	  - 类型：net.Error（含 *net.OpError / *url.Error）、context.Canceled、
//	    context.DeadlineExceeded、io.EOF / io.ErrUnexpectedEOF、net.ErrClosed、
//	    syscall.ECONNREFUSED / ECONNRESET / EPIPE / ETIMEDOUT / EHOSTUNREACH / ENETUNREACH
//	  - 文本兜底：dial tcp、connection refused/reset、i/o timeout、no such host、
//	    broken pipe、EOF、context deadline exceeded、TLS handshake timeout 等
//
//	ErrorKindNotReady（未就绪/软错误，保持原有重试语义，不计入跳过）：
//	  - *mix.Warn（本仓用于「数据还未同步完毕」之类的软失败，例如
//	    injector/setTracesBuilder 的「agg trace 未同步完毕」）；不走异常路径，不跳过。
//	  - 聚合器返回 code:2（impl.ErrNotFound）属于此类：语义是「agg 还没索引到该高度」，
//	    跳过会造成真实数据永久丢失，因此保守地按未就绪处理（保持重试）。
//
//	ErrorKindUnknown（其它未知错误）：不计入跳过（保守：只对能判定为数据级的错误动手）。
type ErrorKind int

const (
	ErrorKindUnknown ErrorKind = iota
	ErrorKindData
	ErrorKindTransport
	ErrorKindNotReady
)

func (k ErrorKind) String() string {
	switch k {
	case ErrorKindData:
		return "data"
	case ErrorKindTransport:
		return "transport"
	case ErrorKindNotReady:
		return "not-ready"
	default:
		return "unknown"
	}
}

// ClassifySyncError 按上面的判据给错误分类
func ClassifySyncError(err error) ErrorKind {
	if err == nil {
		return ErrorKindUnknown
	}

	// 1. 传输级优先：网络坏 ≠ 数据坏，宁可重试也不要误跳
	if isTransportError(err) {
		return ErrorKindTransport
	}

	// 2. 数据级（类型/错误码判据）
	var bizErr *londobell.BusinessError
	if errors.As(err, &bizErr) {
		if isTransportText(bizErr.Message) || isTransportText(bizErr.URL) {
			return ErrorKindTransport
		}
		return ErrorKindData
	}
	var decodeErr *londobell.DecodeError
	if errors.As(err, &decodeErr) {
		return ErrorKindData
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		return ErrorKindData
	}

	// 3. 未就绪/软错误
	var warn *mix.Warn
	if errors.As(err, &warn) {
		return ErrorKindNotReady
	}

	// 4. 文本兜底（错误链里没有任何可识别的类型时）
	if isTransportText(err.Error()) {
		return ErrorKindTransport
	}

	return ErrorKindUnknown
}

// isTransportError 传输级错误的类型判据
func isTransportError(err error) bool {
	if errors.Is(err, context.Canceled) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EPIPE) ||
		errors.Is(err, syscall.ETIMEDOUT) ||
		errors.Is(err, syscall.EHOSTUNREACH) ||
		errors.Is(err, syscall.ENETUNREACH) {
		return true
	}
	// *net.OpError / *url.Error 均实现 net.Error
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return true
	}
	return false
}

// transportTexts 传输级错误的文本兜底判据（无类型信息的历史错误、被回透成业务码的网络错误）
var transportTexts = []string{
	"dial tcp",
	"dial udp",
	"connection refused",
	"connection reset",
	"broken pipe",
	"i/o timeout",
	"no such host",
	"network is unreachable",
	"tls handshake timeout",
	"timeout awaiting response headers",
	"client.timeout exceeded",
	"context deadline exceeded",
	"context canceled",
	"use of closed network connection",
	"temporary failure in name resolution",
	"EOF",
}

// isTransportText 文本兜底判据
func isTransportText(s string) bool {
	if s == "" {
		return false
	}
	for _, v := range transportTexts {
		if strings.Contains(s, v) {
			return true
		}
	}
	return false
}

// errorSummary 生成台账/日志用的错误摘要（按字符截断）
func errorSummary(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	runes := []rune(msg)
	if len(runes) <= maxDataErrorMessageLen {
		return msg
	}
	return string(runes[:maxDataErrorMessageLen]) + " ...(truncated)"
}

// dataErrorFailure 单个高度上的失败进度（内存态，进程重启后重新累计）
type dataErrorFailure struct {
	failures int64
	firstAt  time.Time
	lastAt   time.Time
	err      error

	identity *po.SyncSyncerEpoch // 判定跳过时回填的 tipset 身份行（保持链条连续）
	skipped  bool                // 已登记跳过
	warned   bool                // 跳过后的短路径告警只打一次，避免刷屏
}

// dataErrorTracker 按高度记录数据级错误次数，达到阈值即判定「可跳过」。
//
// 计数语义：同一高度在**未被成功同步**期间，累计出现的数据级错误失败次数。
// 非数据级错误（传输级/未就绪/未知）既不计入也不清零 —— 它们既不能证明该高度数据
// 已可用，也不能证明已损坏（避免聚合器网络抖动把计数清零，导致再次出现静默停摆）。
type dataErrorTracker struct {
	threshold int64
	mu        sync.Mutex
	items     map[int64]*dataErrorFailure
}

func newDataErrorTracker(threshold int64) *dataErrorTracker {
	if threshold <= 0 {
		threshold = defaultDataErrorThreshold
	}
	return &dataErrorTracker{
		threshold: threshold,
		items:     map[int64]*dataErrorFailure{},
	}
}

// reach 记录一次数据级错误失败，返回记录副本与「是否达到跳过阈值」
func (t *dataErrorTracker) reach(epoch chain.Epoch, err error) (rec dataErrorFailure, reached bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	item, ok := t.items[epoch.Int64()]
	if !ok {
		item = &dataErrorFailure{firstAt: time.Now()}
		t.items[epoch.Int64()] = item
	}
	item.failures++
	item.lastAt = time.Now()
	item.err = err

	return *item, item.failures >= t.threshold && !item.skipped
}

// get 返回记录副本（不存在返回 false）
func (t *dataErrorTracker) get(epoch chain.Epoch) (rec dataErrorFailure, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	item, exists := t.items[epoch.Int64()]
	if !exists {
		return dataErrorFailure{}, false
	}
	return *item, true
}

// markSkipped 标记该高度已登记跳过（并回填 tipset 身份行）
func (t *dataErrorTracker) markSkipped(epoch chain.Epoch, identity *po.SyncSyncerEpoch) {
	t.mu.Lock()
	defer t.mu.Unlock()

	item, ok := t.items[epoch.Int64()]
	if !ok {
		item = &dataErrorFailure{firstAt: time.Now(), lastAt: time.Now()}
		t.items[epoch.Int64()] = item
	}
	item.skipped = true
	item.identity = identity
}

// markWarned 标记该高度已打过短路径告警，返回是否是首次（true 表示本次由调用方负责打印）
func (t *dataErrorTracker) markWarned(epoch chain.Epoch) (first bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	item, ok := t.items[epoch.Int64()]
	if !ok {
		return false
	}
	if item.warned {
		return false
	}
	item.warned = true
	return true
}

// reset 清空全部状态（链回滚后，被丢弃高度的判定不再成立，需要重新累计）
func (t *dataErrorTracker) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.items = map[int64]*dataErrorFailure{}
}

// resetBelow 清理低于 epoch 的高度状态（这些高度已成功推进过去，计数不再需要）
func (t *dataErrorTracker) resetBelow(epoch chain.Epoch) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for k := range t.items {
		if k < epoch.Int64() {
			delete(t.items, k)
		}
	}
}

// recordEpochFailure 记录某高度的一次执行失败，并在判定为「数据级坏点」时登记跳过。
// 返回 true 表示该高度已被登记跳过（调用方无需再按失败重试该高度）。
func (s *Syncer) recordEpochFailure(epoch chain.Epoch, err error) (skipped bool) {
	if s.dry || err == nil || s.failures == nil {
		return false
	}

	// 已经登记为数据级坏点：不重复计数、不重复登记（同一高度只登一次台账）
	if _, ok := s.skippedEpochFailure(epoch); ok {
		return true
	}

	// 只有数据级错误才累计（判据见 ClassifySyncError）：传输级/未就绪/未知错误
	// 一律保持原有的重试语义，绝不跳过。
	if ClassifySyncError(err) != ErrorKindData {
		return false
	}

	rec, reached := s.failures.reach(epoch, err)
	if !reached {
		s.log.Warnf("高度 %s 数据级错误失败: 第 %d 次（连续 %d 次失败即登记并跳过）, 错误: %s",
			epoch, rec.failures, s.dataErrorThreshold, errorSummary(err))
		return false
	}

	return s.skipEpoch(epoch, rec)
}

// skipEpoch 登记并跳过某高度。
//
// 三个前置条件任一不满足就**不跳过**（保持原有重试语义并打 ERROR，绝不静默）：
//  1. 能取到该高度的 tipset 身份 —— 否则 chain.sync_syncer_epochs 会出现空洞，
//     CheckBlockChainConsistency 沿 parent_keys 回溯时会把空洞当成链分叉并触发回滚，
//     反而制造「回滚-重试」死循环；
//  2. 该身份行写入成功 —— 同上，跳过必须保证链条连续；
//  3. 台账写入成功 —— 不能留痕就不算登记，不允许跳过。
func (s *Syncer) skipEpoch(epoch chain.Epoch, rec dataErrorFailure) (ok bool) {
	ctx := context.Background()
	summary := errorSummary(rec.err)

	if s.skipLedger == nil {
		s.log.Errorf("高度 %s 连续 %d 次数据级错误失败，但跳过台账未初始化，不允许无留痕地跳过，继续按原逻辑重试",
			epoch, rec.failures)
		return false
	}

	identity, err := s.prepareSyncerEpoch(epoch)
	if err != nil {
		s.log.Errorf("高度 %s 连续 %d 次数据级错误失败，但无法获取该高度 tipset 身份，不能安全跳过（跳过会造成链条空洞并触发误判回滚），继续按原逻辑重试: %s",
			epoch, rec.failures, err)
		return false
	}
	// 先写身份行、后写台账：顺序不能反 —— 台账一旦登记成功，跳过就生效了，
	// 若此时身份行还没写，链条就有空洞（比停摆更糟）；反过来若身份行先落库而台账写失败，
	// 只是多一条真实身份的 tipset 行（该高度下轮仍会正常重试），无副作用。
	if !s.dry {
		if err = s.repo.SaveSyncSyncerEpoch(ctx, identity); err != nil {
			s.log.Errorf("高度 %s 连续 %d 次数据级错误失败，但 tipset 身份行写入失败，不能安全跳过，继续按原逻辑重试: %s",
				epoch, rec.failures, err)
			return false
		}
	}

	item := &po.SyncSkippedEpoch{
		Syncer:        s.name,
		Epoch:         epoch.Int64(),
		ErrorMessage:  summary,
		Failures:      rec.failures,
		FirstFailedAt: rec.firstAt,
		LastFailedAt:  rec.lastAt,
	}
	if err = s.skipLedger.SaveSkippedEpoch(ctx, item); err != nil {
		s.log.Errorf("高度 %s 连续 %d 次数据级错误失败，但跳过台账(chain.sync_skipped_epochs)写入失败，"+
			"不允许无留痕地跳过（请确认 migration/33.sync_skipped_epochs.sql 已执行），继续按原逻辑重试: %s",
			epoch, rec.failures, err)
		return false
	}

	s.failures.markSkipped(epoch, identity)
	s.log.Errorf("高度 %s 连续 %d 次数据级错误失败，已跳过该高度并登记台账 chain.sync_skipped_epochs（后续高度继续同步，该高度需人工确认/重跑）, 错误: %s",
		epoch, rec.failures, summary)
	return true
}

// skippedEpochFailure 返回该高度是否已被登记跳过（含 tipset 身份行）
func (s *Syncer) skippedEpochFailure(epoch chain.Epoch) (rec dataErrorFailure, ok bool) {
	if s.failures == nil {
		return dataErrorFailure{}, false
	}
	rec, ok = s.failures.get(epoch)
	if !ok || !rec.skipped {
		return dataErrorFailure{}, false
	}
	return rec, true
}

// logSkippedEpochOnce 跳过后的短路径告警，同一高度只打一次，避免刷屏
func (s *Syncer) logSkippedEpochOnce(epoch chain.Epoch) {
	if s.failures == nil {
		return
	}
	if !s.failures.markWarned(epoch) {
		return
	}
	s.log.Warnf("高度 %s 是已登记的数据级坏点（跳过台账 chain.sync_skipped_epochs），本次不执行其任务与计算器，仅回填 tipset 身份行",
		epoch)
}

// saveSkippedEpochIdentity 跳过的高度不执行任务与计算器，但仍写入 tipset 身份行，
// 保证 chain.sync_syncer_epochs 沿 parent_keys 连续（否则一致性检查会误判分叉）。
func (s *Syncer) saveSkippedEpochIdentity(epoch chain.Epoch, item *po.SyncSyncerEpoch) (err error) {
	if s.dry {
		return nil
	}
	if item == nil {
		item, err = s.prepareSyncerEpoch(epoch)
		if err != nil {
			return err
		}
	}
	return s.repo.SaveSyncSyncerEpoch(context.Background(), item)
}
