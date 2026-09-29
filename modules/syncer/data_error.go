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

// 本文件实现两类「连续 N 次失败 ⇒ 跳过并登记台账」，两类**各自独立计数、独立阈值**：
//
//	① 数据级错误（ErrorKindData）：聚合器对某高度返回业务码 code:1（链上第三方提交的参数
//	   无法序列化）或响应体解码失败等。连续 defaultDataErrorThreshold 次 ⇒ 跳过该高度并登记。
//
//	② 节点侧状态不可用（ErrorKindUnrecoverableState，不可恢复）：同步器逐高度同步时必须读该高度的
//	   链上状态树，而本地 lotus 只保留近期窗口，历史状态已被裁掉 —— 日志形如
//	   「load state tree: failed to load state tree bafy...: failed to load hamt node: ...」
//	   /「ipld: could not find ...」。连续 defaultUnrecoverableErrorThreshold 次 ⇒ 登记跳过；
//	   并且当「链头 − 当前高度」达到 defaultStateGapJumpMinGap 时，**一次跳过整个区间**
//	   [当前高度, 链头 − defaultStateGapJumpMargin]（实测缺口是几万个连续高度，逐个跳过毫无意义），
//	   区间行写入 chain.sync_skipped_epochs（见 migration/34.sync_skipped_epochs_ranges.sql）。
//
// 两类之外（传输级/未就绪/未知）既不计入也不清零，保持原有「失败即重试」语义。
//
// 回补办法（可逆性）：区间跳只是把 chain.sync_syncers.epoch 指针推到「链头 − margin」，并把被跳过
// 的区间记在 chain.sync_skipped_epochs（skipped_from / skipped_to）里；节点状态恢复后，把
// chain.sync_syncers.epoch 改回区间起点（skipped_from）并重启同步器即可重跑该区间，
// 重跑成功后删掉对应区间行（详见 migration/34.sync_skipped_epochs_ranges.sql 的说明）。

const (
	// defaultDataErrorThreshold 数据级错误连续失败次数的默认阈值：
	// 同一高度连续 5 轮都是数据级失败 ⇒ 登记并跳过该高度。
	// 配置项：syncer.data_error_threshold（见 modules/common/config/config.go）
	defaultDataErrorThreshold int64 = 5

	// defaultUnrecoverableErrorThreshold 节点侧状态不可用（不可恢复）错误连续失败次数的默认阈值：
	// 同一高度连续 20 轮都是这一类失败 ⇒ 登记并跳过。重试间隔默认 15s ⇒ 约 5 分钟后动手，
	// 目的是把「节点重启/状态窗口瞬时不可用」与「历史状态已被裁掉」区分开。
	// 配置项：syncer.unrecoverable_error_threshold
	defaultUnrecoverableErrorThreshold int64 = 20

	// defaultStateGapJumpMinGap 触发「一次跳过整段不可恢复区间」的最小缺口（链头 − 当前高度）：
	// 缺口小于它说明只是零星坏高度，按原逻辑重试/单高度跳过即可，不做区间跳。
	//
	// 2026-09-29 由 1000 抬高到 5000。原因（主网实测）：miner / pro / sector 这类任务必须等
	// 「链头 − 约 1000」（EC finality）才写数据，正常运行时它们本来就停在链头下方约 1000 处；
	// 阈值 1000 与这个正常落差贴死 ⇒ 一旦短暂卡顿就越过门槛，把「本可恢复」的区间误判为
	// 「不可恢复」并一次性跳过。实测：sector-task 于 09-24 09:24 在链头 −1046 处误跳，
	// 留出 1045 高度的台账空洞。抬高后判定整段不可恢复要多等一会儿，代价只是延迟，不会丢数据。
	// 配置项：syncer.state_gap_jump_min_gap（生产可在 config.toml 覆盖本默认值）
	defaultStateGapJumpMinGap int64 = 5000

	// defaultStateGapJumpMargin 区间跳的目标高度 = 链头 − margin（留出安全边界，避免贴着链头被回滚）。
	// 配置项：syncer.state_gap_jump_margin
	defaultStateGapJumpMargin int64 = 200

	// maxDataErrorMessageLen 台账中错误信息的最大长度（按字符截断，防止超长错误撑爆存储）
	maxDataErrorMessageLen = 1000
)

// 台账 chain.sync_skipped_epochs.error_class 的取值（老记录为 NULL，语义等同 errorClassData）。
const (
	errorClassData               = "data"                // 数据级错误（示例：聚合器 code:1 / 响应体解码失败）
	errorClassUnrecoverableState = "unrecoverable-state" // 节点侧历史状态不可用（不可恢复）
)

// 区间跳的限频与文案常量
const (
	// gapWarnInterval 「未达区间跳门槛」告警的最小间隔（避免每 15s 刷屏）
	gapWarnInterval = time.Minute
)

// errorClassLabel 台账/日志里的中文类名
func errorClassLabel(class string) string {
	switch class {
	case errorClassUnrecoverableState:
		return "节点侧不可恢复(历史状态不可用)错误"
	default:
		return "数据级错误"
	}
}

func ptrString(v string) *string { return &v }

func ptrInt64(v int64) *int64 { return &v }

// ErrorKind 同步器错误的分类，决定「该高度能否被跳过」。
//
// 判据（改分类前先读这里，判据优先级：类型 > 错误码 > 文本兜底）：
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
//	ErrorKindUnrecoverableState（节点侧不可恢复，可计数、达阈值可跳过 + 缺口足够大时区间跳）：
//	  - 文本判据（见 unrecoverableStateTexts）：load state tree、failed to load hamt node、
//	    failed to load state、ipld: could not find、blockstore get。
//	  - 语义：该高度的链上状态树在节点侧已不可用（本地只保留近期窗口，历史状态被裁掉），
//	    重试同一高度不会自愈，且实测缺口是几万个连续高度。
//	  - **优先级**：传输级与未就绪类都排在本类之前 —— 网络错误/节点未就绪时恰好带
//	    ipld 字样（例如把上游网络错误文本回透）绝不能归到本类（否则会把「暂时不通」当成
//	    「历史状态已裁掉」而跳掉一大段真实高度）。因此本类的文本判据命中后，还要再剔除
//	    命中传输级文本兜底的情形（见 ClassifySyncError 第 3 步）。
//
//	ErrorKindData（数据级，可计数、达阈值可跳过）：
//	  1. *londobell.BusinessError：聚合器/适配器 HTTP 200 但响应体 code != 0（且非 2）
//	     （例如 code:1）。同一高度重试不会自愈 —— 服务端明确处理了这次请求并失败。
//	     例外：若其 Message 命中传输级文本兜底（服务端把下游网络错误回透成业务码），
//	     一律按传输级处理，避免误跳；若其 Message 命中节点侧不可恢复判据（上游把
//	     load state tree 回透成业务码），按节点侧不可恢复处理（第 3 步已兜住）。
//	  2. *londobell.DecodeError：响应体存在但 json 解码失败。
//	  3. encoding/json 的 *json.SyntaxError / *json.UnmarshalTypeError（上游未包装时）。
//
//	ErrorKindUnknown（其它未知错误）：不计入跳过（保守：只对能判定类别的错误动手）。
type ErrorKind int

const (
	ErrorKindUnknown ErrorKind = iota
	ErrorKindData
	ErrorKindTransport
	ErrorKindNotReady
	// ErrorKindUnrecoverableState 追加在末尾：不改动既有枚举值，避免影响外部按值比较的场景
	ErrorKindUnrecoverableState
)

func (k ErrorKind) String() string {
	switch k {
	case ErrorKindData:
		return "data"
	case ErrorKindTransport:
		return "transport"
	case ErrorKindNotReady:
		return "not-ready"
	case ErrorKindUnrecoverableState:
		return errorClassUnrecoverableState
	default:
		return "unknown"
	}
}

// ClassifySyncError 按上面的判据给错误分类。
//
// 分类顺序（改动前务必保持这个优先级，并同步更新 data_error_test.go 的正/误判用例）：
//
//  1. 传输级（类型判据）——最高优先级：网络坏 ≠ 数据坏，宁可重试也不要误跳；
//  2. 未就绪/软错误（*mix.Warn、业务码 code:2）——语义是「还没到/还没索引到」，重试即可；
//  3. 数据级（类型/错误码判据）——但上游可能把「节点侧状态不可用」回透成业务码 code:1 /
//     解码失败，此时语义仍是「节点侧没有这段历史状态」，所以本步内先按文本判据把这类错误
//     分到 ErrorKindUnrecoverableState（网络文本仍优先：先剔除传输级文本）；
//  4. 文本兜底（错误链里没有任何可识别的类型时）：网络 > 节点侧状态 > 未知。
//
// 实现约束：**先按类型判断，再读 Error() 文本**（符合「类型 > 错误码 > 文本兜底」），
// 且所有文本读取都走 safeErrorText —— 个别标准库错误（如零值 *json.UnmarshalTypeError）
// 的 Error() 会 panic，不能在类型判定之前触碰。
func ClassifySyncError(err error) ErrorKind {
	if err == nil {
		return ErrorKindUnknown
	}

	// 1. 传输级优先：网络坏 ≠ 数据坏，宁可重试也不要误跳
	if isTransportError(err) {
		return ErrorKindTransport
	}

	// 2. 未就绪/软错误优先于「节点侧不可恢复」：Warn（未同步完毕）/ code:2（上游还没索引到）
	if isNotReadyError(err) {
		return ErrorKindNotReady
	}

	// 3. 数据级（类型/错误码判据）；先让「节点侧不可恢复」的文本判据在本类内部否决
	var bizErr *londobell.BusinessError
	if errors.As(err, &bizErr) {
		if isTransportText(bizErr.Message) || isTransportText(bizErr.URL) {
			return ErrorKindTransport
		}
		if isUnrecoverableStateText(bizErr.Message) {
			return ErrorKindUnrecoverableState
		}
		return ErrorKindData
	}
	var decodeErr *londobell.DecodeError
	if errors.As(err, &decodeErr) {
		if isUnrecoverableStateText(safeErrorText(err)) {
			return ErrorKindUnrecoverableState
		}
		return ErrorKindData
	}
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntaxErr) || errors.As(err, &typeErr) {
		// 注意：这里不读 Error()（零值 *json.UnmarshalTypeError 的 Error() 会 panic）
		return ErrorKindData
	}

	// 4. 文本兜底（错误链里没有任何可识别的类型时）：网络优先于节点侧状态，最后才是未知
	text := safeErrorText(err)
	if isTransportText(text) {
		return ErrorKindTransport
	}
	if isUnrecoverableStateText(text) {
		return ErrorKindUnrecoverableState
	}

	return ErrorKindUnknown
}

// safeErrorText 读取错误文本，并在 Error() panic 时退化成空串。
//
// 理由：错误文本是第三方实现，个别错误类型在零值/残缺字段上会 panic
// （实测 encoding/json 的零值 *json.UnmarshalTypeError 就会 SIGSEGV）；
// 分类器只做「兜底归类」，绝不能因为一个畸形错误把同步器打挂。
func safeErrorText(err error) (text string) {
	if err == nil {
		return ""
	}
	defer func() {
		if recover() != nil {
			text = ""
		}
	}()
	return err.Error()
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

// isNotReadyError 未就绪/软错误的类型/错误码判据（优先级高于「节点侧不可恢复」）
func isNotReadyError(err error) bool {
	// 本仓软失败（例如 injector/setTracesBuilder 的「agg trace 未同步完毕」）
	var warn *mix.Warn
	if errors.As(err, &warn) {
		return true
	}
	// 业务码 code:2：上游还没索引到该高度（客户端实现通常已把它转成 impl.ErrNotFound 哨兵，
	// 哨兵文本不含状态判据、会落到 ErrorKindUnknown，同样「不计入也不跳过」，这里兜住未转换的路径）
	var bizErr *londobell.BusinessError
	if errors.As(err, &bizErr) && bizErr.Code == londobell.CodeNotFound {
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

// unrecoverableStateTexts 节点侧「历史状态不可用（不可恢复）」错误的文本判据。
//
// 取自线上实测日志（2026-09 主网同步器卡死 18 天）：
//
//	执行 ContextBuilder 错误: load state tree: failed to load state tree bafy2bzacea72i…:
//	failed to load hamt node: …
//
// 上游（聚合器/节点）没有结构化错误码，只能按文本归类；判据尽量取「带上下文的长片段」，
// 避免单字（如 "state"）误伤无关错误。
var unrecoverableStateTexts = []string{
	"load state tree",
	"failed to load hamt node",
	"failed to load state",
	"ipld: could not find",
	"blockstore get",
}

// isUnrecoverableStateText 节点侧不可恢复错误的文本判据
func isUnrecoverableStateText(s string) bool {
	if s == "" {
		return false
	}
	for _, v := range unrecoverableStateTexts {
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

// dataErrorTracker 按高度记录「某一类错误」的连续失败次数，达到阈值即判定「可跳过」。
//
// 同一实现被实例化为两个**互相独立**的计数器（见 Syncer.failures / Syncer.unrecoverable）：
// 数据级错误与「节点侧状态不可用」各自计数、各自阈值，互不影响。
//
// 计数语义：同一高度在**未被成功同步**期间，累计出现的该类错误失败次数。
// 其它类别错误（传输级/未就绪/未知）既不计入也不清零 —— 它们既不能证明该高度数据
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

// reach 记录一次该类错误失败，返回记录副本与「是否达到跳过阈值」
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

// reachedInRange 返回 [from, to] 内「已达阈值」的最小高度记录（区间跳判定用）。
//
// 与 reach 的返回值不同：这里**不要求** !skipped —— 达阈值后该高度会先被登记为单高度跳过，
// 之后本批失败返回前仍要据此判断是否把这次跳过升级为「区间跳」。
func (t *dataErrorTracker) reachedInRange(from, to chain.Epoch) (epoch chain.Epoch, rec dataErrorFailure, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if to < from {
		return 0, dataErrorFailure{}, false
	}
	for k, item := range t.items {
		if k < from.Int64() || k > to.Int64() {
			continue
		}
		if item.failures < t.threshold {
			continue
		}
		if !ok || k < epoch.Int64() {
			epoch, rec, ok = chain.Epoch(k), *item, true
		}
	}
	return
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

// trackers 返回所有失败计数器（数据级 + 节点侧不可恢复），二者独立计数、互不影响
func (s *Syncer) trackers() []*dataErrorTracker {
	var ts []*dataErrorTracker
	if s.failures != nil {
		ts = append(ts, s.failures)
	}
	if s.unrecoverable != nil {
		ts = append(ts, s.unrecoverable)
	}
	return ts
}

// resetFailures 清空所有类别的失败计数与跳过状态（链回滚后调用）
func (s *Syncer) resetFailures() {
	for _, t := range s.trackers() {
		t.reset()
	}
}

// resetFailuresBelow 清理低于该高度的失败计数（该批高度已推进过去）
func (s *Syncer) resetFailuresBelow(epoch chain.Epoch) {
	for _, t := range s.trackers() {
		t.resetBelow(epoch)
	}
}

// recordEpochFailure 记录某高度的一次执行失败，并在判定为「坏点」时登记跳过。
// 返回 true 表示该高度已被登记跳过（调用方无需再按失败重试该高度）。
//
// 分类决定走向（判据见 ClassifySyncError）：
//   - 数据级 → 计入 s.failures（阈值 syncer.data_error_threshold），达阈值单高度跳过；
//   - 节点侧不可恢复 → 计入 s.unrecoverable（阈值 syncer.unrecoverable_error_threshold），
//     达阈值单高度跳过；区间跳由 sync() 在本批失败返回前判定（见 tryStateGapJump）；
//   - 传输级/未就绪/未知 → 不计入也不清零，保持原有重试语义。
func (s *Syncer) recordEpochFailure(epoch chain.Epoch, err error) (skipped bool) {
	if s.dry || err == nil {
		return false
	}

	// 已经登记为坏点：不重复计数、不重复登记（同一高度只登一次台账）
	if _, ok := s.skippedEpochFailure(epoch); ok {
		return true
	}

	switch ClassifySyncError(err) {
	case ErrorKindData:
		return s.recordFailure(epoch, err, s.failures, errorClassData)
	case ErrorKindUnrecoverableState:
		return s.recordFailure(epoch, err, s.unrecoverable, errorClassUnrecoverableState)
	default:
		// 传输级/未就绪/未知：保持原有重试语义，绝不计入、绝不清零
		return false
	}
}

// recordFailure 某一类错误的「连续失败计数 + 达阈值登记跳过」公共路径
func (s *Syncer) recordFailure(epoch chain.Epoch, err error, tracker *dataErrorTracker, class string) (skipped bool) {
	if tracker == nil {
		return false
	}

	rec, reached := tracker.reach(epoch, err)
	if !reached {
		s.log.Warnf("高度 %s %s失败: 第 %d 次（连续 %d 次失败即登记并跳过）, 错误: %s",
			epoch, errorClassLabel(class), rec.failures, tracker.threshold, errorSummary(err))
		return false
	}

	return s.skipEpoch(epoch, rec, tracker, class)
}

// skipEpoch 登记并跳过某高度（单高度语义；区间跳见 applyStateGapJump）。
//
// 三个前置条件任一不满足就**不跳过**（保持原有重试语义并打 ERROR，绝不静默）：
//  1. 能取到该高度的 tipset 身份 —— 否则 chain.sync_syncer_epochs 会出现空洞，
//     CheckBlockChainConsistency 沿 parent_keys 回溯时会把空洞当成链分叉并触发回滚，
//     反而制造「回滚-重试」死循环；
//  2. 该身份行写入成功 —— 同上，跳过必须保证链条连续；
//  3. 台账写入成功 —— 不能留痕就不算登记，不允许跳过。
//
// class 写入台账 error_class；跳过状态登记在 tracker（该类错误的独立计数器）上。
func (s *Syncer) skipEpoch(epoch chain.Epoch, rec dataErrorFailure, tracker *dataErrorTracker, class string) (ok bool) {
	ctx := context.Background()
	summary := errorSummary(rec.err)
	label := errorClassLabel(class)

	if s.skipLedger == nil {
		s.log.Errorf("高度 %s 连续 %d 次%s失败，但跳过台账未初始化，不允许无留痕地跳过，继续按原逻辑重试",
			epoch, rec.failures, label)
		return false
	}

	identity, err := s.prepareSyncerEpoch(epoch)
	if err != nil {
		s.log.Errorf("高度 %s 连续 %d 次%s失败，但无法获取该高度 tipset 身份，不能安全跳过（跳过会造成链条空洞并触发误判回滚），继续按原逻辑重试: %s",
			epoch, rec.failures, label, err)
		return false
	}
	// 先写身份行、后写台账：顺序不能反 —— 台账一旦登记成功，跳过就生效了，
	// 若此时身份行还没写，链条就有空洞（比停摆更糟）；反过来若身份行先落库而台账写失败，
	// 只是多一条真实身份的 tipset 行（该高度下轮仍会正常重试），无副作用。
	if !s.dry && s.repo != nil {
		if err = s.repo.SaveSyncSyncerEpoch(ctx, identity); err != nil {
			s.log.Errorf("高度 %s 连续 %d 次%s失败，但 tipset 身份行写入失败，不能安全跳过，继续按原逻辑重试: %s",
				epoch, rec.failures, label, err)
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
		ErrorClass:    ptrString(class),
	}
	if err = s.skipLedger.SaveSkippedEpoch(ctx, item); err != nil {
		s.log.Errorf("高度 %s 连续 %d 次%s失败，但跳过台账(chain.sync_skipped_epochs)写入失败，"+
			"不允许无留痕地跳过（请确认 migration/33.sync_skipped_epochs.sql 已执行），继续按原逻辑重试: %s",
			epoch, rec.failures, label, err)
		return false
	}

	tracker.markSkipped(epoch, identity)
	s.log.Errorf("高度 %s 连续 %d 次%s失败，已跳过该高度并登记台账 chain.sync_skipped_epochs(error_class=%s)（后续高度继续同步，该高度需人工确认/重跑）, 错误: %s",
		epoch, rec.failures, label, class, summary)
	return true
}

// stateGapJumpPlan 「一次跳过整段不可恢复区间」的计划。
//
// 被跳过的高度区间 = [From, Next-1]（含两端），Next 是跳完之后的新同步起点。
type stateGapJumpPlan struct {
	From chain.Epoch // 区间起点（当前高度，也是台账行的 epoch / skipped_from）
	Next chain.Epoch // 新的同步起点 = 链头 − margin
	Head chain.Epoch // 链头（agg final height，与运维人工跳指针时的「链头」同口径）
}

// Last 被跳过区间的终点（含）
func (p stateGapJumpPlan) Last() chain.Epoch { return p.Next - 1 }

// Count 被跳过区间的高度个数
func (p stateGapJumpPlan) Count() int64 { return p.Next.Int64() - p.From.Int64() }

// planStateGapJump 区间跳的数学判据（纯函数，便于单测）。全部满足才跳：
//  1. minGap > 0 且 margin >= 0（非法配置不跳）；
//  2. 链头严格领先当前高度（head > from）；
//  3. 缺口（head − from）≥ minGap —— 缺口不够大时只按原逻辑重试/单高度跳过，不许区间跳；
//  4. 目标高度（head − margin）**严格大于**当前高度 —— margin ≥ 缺口时不跳，
//     否则「跳」只会原地退步或让缺口变大。
func planStateGapJump(from, head chain.Epoch, minGap, margin int64) (stateGapJumpPlan, bool) {
	if minGap <= 0 || margin < 0 {
		return stateGapJumpPlan{}, false
	}
	if head <= from {
		return stateGapJumpPlan{}, false
	}
	if head.Int64()-from.Int64() < minGap {
		return stateGapJumpPlan{}, false
	}
	next := head - chain.Epoch(margin)
	if next <= from {
		return stateGapJumpPlan{}, false
	}
	return stateGapJumpPlan{From: from, Next: next, Head: head}, true
}

// tryStateGapJump 判定并执行「区间跳」（由 sync() 在本批失败返回前调用，单协程，不并发）。
//
// 判定：本批内存在「节点侧不可恢复错误」连续失败达阈值的高度（独立计数器），
// 且该高度到链头的缺口 ≥ syncer.state_gap_jump_min_gap ⇒ 一次跳到 链头 − margin。
// 缺口不够大 ⇒ 什么都不做（保持原逻辑重试/单高度跳过，绝不区间跳）。
//
// 返回 true 表示已完成区间跳（s.epoch 已被推进到新区间起点）。
func (s *Syncer) tryStateGapJump(head chain.Epoch) (jumped bool) {
	if s.unrecoverable == nil {
		return false
	}
	from, _, ok := s.unrecoverable.reachedInRange(s.epoch, head)
	if !ok {
		return false
	}

	plan, ok := planStateGapJump(from, head, s.stateGapJumpMinGap, s.stateGapJumpMargin)
	if !ok {
		if s.gapWarnAllowed() {
			s.log.Warnf("高度 %s 节点侧不可恢复错误已连续失败 %d 次，但链头 %s 与当前高度缺口 %d 未达区间跳门槛 %d（或 margin %d 过大），按原逻辑重试/单高度跳过（%s 内不再重复告警）",
				from, s.unrecoverable.threshold, head, head.Int64()-from.Int64(), s.stateGapJumpMinGap, s.stateGapJumpMargin, gapWarnInterval)
		}
		return false
	}
	return s.applyStateGapJump(plan)
}

// gapWarnAllowed 「未达区间跳门槛」告警的限频（默认 60s 一次，避免每 15s 刷屏）
func (s *Syncer) gapWarnAllowed() bool {
	now := time.Now()
	if !s.gapWarnAt.IsZero() && now.Sub(s.gapWarnAt) < gapWarnInterval {
		return false
	}
	s.gapWarnAt = now
	return true
}

// applyStateGapJump 执行区间跳：
//  1. 复核前置（该高度已登记单高度跳过、tipset 身份行已落库、台账可用）——不满足就不跳，
//     保持原有重试语义（拿不到身份/不能留痕时绝不跳）；
//  2. 把台账行升级为区间行：同一 (syncer, epoch) 行 upsert，epoch = skipped_from，
//     并写入 skipped_to / error_class（区间的全部高度都由这一行覆盖）；
//  3. 推进 s.epoch 到新区间起点（链头 − margin），并把同步器进度指针一并写库（重启不退回）；
//  4. ERROR 级日志写清区间、缺口与原因。
func (s *Syncer) applyStateGapJump(plan stateGapJumpPlan) (ok bool) {
	ctx := context.Background()
	from := plan.From

	rec, registered := s.lookupSkipped(from)
	if !registered || rec.identity == nil {
		s.log.Errorf("高度 %s 触发区间跳，但该高度尚未登记 tipset 身份行，不能安全跳过（拿不到身份一律不跳），继续按原逻辑重试", from)
		return false
	}
	if s.skipLedger == nil {
		s.log.Errorf("高度 %s 触发区间跳，但跳过台账未初始化，不允许无留痕地跳过，继续按原逻辑重试", from)
		return false
	}

	item := &po.SyncSkippedEpoch{
		Syncer:        s.name,
		Epoch:         from.Int64(),
		ErrorMessage:  errorSummary(rec.err),
		Failures:      rec.failures,
		FirstFailedAt: rec.firstAt,
		LastFailedAt:  rec.lastAt,
		ErrorClass:    ptrString(errorClassUnrecoverableState),
		SkippedFrom:   ptrInt64(plan.From.Int64()),
		SkippedTo:     ptrInt64(plan.Last().Int64()),
	}
	if err := s.skipLedger.SaveSkippedEpoch(ctx, item); err != nil {
		s.log.Errorf("高度 %s 触发区间跳，但跳过台账(chain.sync_skipped_epochs)写入失败，不允许无留痕地跳过（请确认 "+
			"migration/34.sync_skipped_epochs_ranges.sql 已执行：skipped_from / skipped_to / error_class 三列），继续按原逻辑重试: %s",
			from, err)
		return false
	}

	s.epoch = plan.Next
	if !s.dry && s.repo != nil {
		// 进度指针一并推进：进程重启后 Init 读到的是新区间起点，不会退回区间内重跑。
		// 写失败不影响本次跳（下一次成功批次仍会覆写该指针），但重启后可能需人工跳指针。
		if err := s.repo.SaveSyncer(ctx, &po.SyncSyncer{Name: s.name, Epoch: plan.Next.Int64() - 1}); err != nil {
			s.log.Warnf("区间跳已生效，但同步器进度指针(chain.sync_syncers, syncer=%s, epoch=%s)写入失败，"+
				"重启后可能需要人工跳指针: %s", s.name, plan.Next-1, err)
		}
	}

	s.log.Errorf("高度 %s 节点侧不可恢复错误（该高度的链上状态在节点侧不可用：load state tree / failed to load hamt node / ipld: could not find）"+
		"连续 %d 次失败，链头 %s 与当前高度缺口 %d ≥ 区间跳门槛 %d，已一次跳过整个区间 [%s, %s]（共 %d 个高度），"+
		"同步起点推进到 %s（= 链头 − %d）；区间已登记台账 chain.sync_skipped_epochs(epoch=%s, skipped_from=%s, skipped_to=%s, error_class=%s)，"+
		"该区间需在节点历史状态恢复后按台账回补（把 chain.sync_syncers.epoch 改回 %s 并重启同步器）, 错误: %s",
		from, rec.failures, plan.Head, plan.Head.Int64()-from.Int64(), s.stateGapJumpMinGap,
		plan.From, plan.Last(), plan.Count(), plan.Next, s.stateGapJumpMargin,
		plan.From, plan.From, plan.Last(), errorClassUnrecoverableState, plan.From,
		errorSummary(rec.err))
	return true
}

// coveredEpochs 把跳过台账记录展开成 [begin, end] 内被覆盖的高度集合。
//
// 单高度行（skipped_from/skipped_to 为 NULL）只贡献 epoch 自身；
// 区间行（区间跳）贡献 [skipped_from, skipped_to] 与查询范围的交集 —— 交集有界，
// 不会因为区间有几万个高度而失控（只在运维回退指针重跑区间时才会命中）。
func coveredEpochs(items []*po.SyncSkippedEpoch, begin, end chain.Epoch) map[int64]struct{} {
	covered := map[int64]struct{}{}
	for _, v := range items {
		if v == nil {
			continue
		}
		from, to := v.Epoch, v.Epoch
		if v.SkippedFrom != nil {
			from = *v.SkippedFrom
		}
		if v.SkippedTo != nil {
			to = *v.SkippedTo
		}
		if from < begin.Int64() {
			from = begin.Int64()
		}
		if to > end.Int64() {
			to = end.Int64()
		}
		for i := from; i <= to; i++ {
			covered[i] = struct{}{}
		}
	}
	return covered
}

// lookupSkipped 返回该高度在任一计数器上被登记跳过的记录
func (s *Syncer) lookupSkipped(epoch chain.Epoch) (rec dataErrorFailure, ok bool) {
	for _, t := range s.trackers() {
		if r, exists := t.get(epoch); exists && r.skipped {
			return r, true
		}
	}
	return dataErrorFailure{}, false
}

// skippedEpochFailure 返回该高度是否已被登记跳过（含 tipset 身份行）
func (s *Syncer) skippedEpochFailure(epoch chain.Epoch) (rec dataErrorFailure, ok bool) {
	return s.lookupSkipped(epoch)
}

// logSkippedEpochOnce 跳过后的短路径告警，同一高度只打一次，避免刷屏
func (s *Syncer) logSkippedEpochOnce(epoch chain.Epoch) {
	first := false
	for _, t := range s.trackers() {
		if r, ok := t.get(epoch); ok && r.skipped && t.markWarned(epoch) {
			first = true
		}
	}
	if !first {
		return
	}
	s.log.Warnf("高度 %s 是已登记的坏点（跳过台账 chain.sync_skipped_epochs），本次不执行其任务与计算器，仅回填 tipset 身份行",
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
