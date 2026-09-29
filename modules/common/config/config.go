package config

type Config struct {
	APIAddress            *string    `toml:"api_address"`         // API 监听地址
	ABIDecoderAddress     *string    `toml:"abi_decoder_address"` // ABI 解码器监听地址
	ABIDecoderRPC         *string    `toml:"abi_decoder_rpc"`     // ABI 解码器 RPC 地址
	ABINode               *string    `toml:"abi_node"`            // ABI 依赖节点
	ABINodeToken          *string    `toml:"abi_node_token"`
	MockMode              bool       `toml:"mock_mode"`       // API Mock 模式
	DSN                   *string    `tom:"dsn"`              // 依赖数据库
	TestNet               bool       `toml:"test_net"`        // 是否为测试网络
	SyncerAddress         *string    `toml:"syncer_address"`  // Syncer 监听地址
	MonitorAddress        *string    `toml:"monitor_address"` // Monitor 监听地址
	Londobell             *Londobell `toml:"londobell"`       // 配置 Londobell 地址
	SyncerTask            bool       `toml:"syncer_task"`     // 开启同步任务
	UpdateCreateTime      bool       `toml:"update_create_time"`
	IpTask                bool       `toml:"ip_task"`                  // 开启 IP 同步
	IgnoreSyncChangeActor *bool      `toml:"ignore_sync_change_actor"` // 忽略同步 ChangeActor，影响账户余额变动
	InitEpoch             *int64     `toml:"init_epoch"`               // 初始化同步器高度
	StopEpoch             *int64     `toml:"stop_epoch"`               // 停止同步高度
	Solidity              *Solidity  `toml:"solidity"`
	Redis                 *Redis     `toml:"redis"`
	Syncer                *Syncer    `toml:"syncer"` // 同步器配置
	Mail                  *Mail      `toml:"mail"`
	ALi                   *ALi       `toml:"ali"`
	Pro                   *Pro       `toml:"pro"`
	// Metrics 「同步落后高度 / 关键表新鲜度」指标采集器（filscan-metrics 命令）配置。
	// 整节可省略：省略时命令使用内置默认值（内置关键表清单 + 127.0.0.1:10020 + 15s）。
	Metrics *Metrics `toml:"metrics"`
	// Feature API 读路径特性开关（整节可省略；省略 = 全部关闭 = 与改造前行为 100% 一致）。
	// 详见 Feature 类型注释与 modules/filscan/biz/browser/agg_pg_reward.go。
	Feature *Feature `toml:"feature"`
}

// Feature API 读路径特性开关。
//
// 语义：这些开关只控制「对外 API 从哪里读这三类统计量」——关闭（零值/缺省）时一律走
// londobell 聚合器（改造前的老路径），开启时改读 PG 派生表（同批数据的预计算结果）。
// 开关关闭时行为与改造前逐字节一致（连一次 SQL 都不会发），因此老配置文件不需要任何改动。
//
// 为什么默认关闭：PG 派生表由同步器写入，其覆盖范围/延迟与聚合器不同源；是否切换必须以
// `filscan-agg-parity` 的两路径一致性校验（同一 miner / 同一高度区间逐字段比对）为准。
type Feature struct {
	// MinerBlockRewardReadFromPg 聚合器端点 miner_blockreward（逐 epoch 出块奖励）
	// 改读 PG 表 chain.miner_rewards。
	MinerBlockRewardReadFromPg *bool `toml:"miner_blockreward_read_from_pg"`
	// MinersBlockRewardReadFromPg 聚合器端点 miners_blockreward（逐 epoch 逐矿工）
	// 改读 PG 表 chain.miner_rewards。
	MinersBlockRewardReadFromPg *bool `toml:"miners_blockreward_read_from_pg"`
	// MinerWinCountReadFromPg 聚合器端点 wincount（逐矿工 winCount + gasReward）
	// 改读 PG 表 chain.miner_win_counts。注意：PG 无 gas_reward 列，开启后
	// TotalGasReward 恒为 0（详见 agg_pg_reward.go 的说明与 parity 工具的 UNRESOLVED 字段）。
	MinerWinCountReadFromPg *bool `toml:"miner_wincount_read_from_pg"`
	// LargeAmountReadFromPg 聚合器端点 transfer_message_for_largeAmount（大额转账列表，FIL>=10000）
	// 改读 PG 表 chain.large_transfers（migration/35.large_transfers.sql）。
	// 注意：该端点只吃 index/limit（没有高度区间），TotalCount 是**全表行数**（不去重）；
	// 同高度内行序线上未定义，PG 侧按 (epoch desc, cid asc) 定序（详见 dal 文件头与
	// agg_pg_large_amount.go）。开启前必须先跑 `filscan-agg-parity -endpoints large_amount`。
	LargeAmountReadFromPg *bool `toml:"large_amount_read_from_pg"`
	// PgReadTimeoutMs 单次 PG 读的超时（毫秒，0 或未配置 = 内置默认 5000ms；<0 = 不设超时）。
	PgReadTimeoutMs *int64 `toml:"pg_read_timeout_ms"`
}

// boolValue 解引用布尔指针，nil 一律视为 false（开关默认关闭，老配置不 panic）。
func boolValue(v *bool) bool {
	return v != nil && *v
}

// MinerBlockRewardReadFromPg 是否让 miner_blockreward 走 PG；未配置 = false（走聚合器）。
func (c *Config) MinerBlockRewardReadFromPg() bool {
	return c != nil && c.Feature != nil && boolValue(c.Feature.MinerBlockRewardReadFromPg)
}

// MinersBlockRewardReadFromPg 是否让 miners_blockreward 走 PG；未配置 = false（走聚合器）。
func (c *Config) MinersBlockRewardReadFromPg() bool {
	return c != nil && c.Feature != nil && boolValue(c.Feature.MinersBlockRewardReadFromPg)
}

// MinerWinCountReadFromPg 是否让 wincount 走 PG；未配置 = false（走聚合器）。
func (c *Config) MinerWinCountReadFromPg() bool {
	return c != nil && c.Feature != nil && boolValue(c.Feature.MinerWinCountReadFromPg)
}

// LargeAmountReadFromPg 是否让大额转账列表走 PG；未配置 = false（走聚合器）。
func (c *Config) LargeAmountReadFromPg() bool {
	return c != nil && c.Feature != nil && boolValue(c.Feature.LargeAmountReadFromPg)
}

// PgReadDefaultTimeoutMs PG 读默认超时（毫秒）。未配置时用它。
const PgReadDefaultTimeoutMs int64 = 5000

// PgReadTimeoutMs 返回配置的 PG 读超时（毫秒）。约定：
//
//	未配置 / Feature 缺失 → PgReadDefaultTimeoutMs（内置默认，不会因为漏配而无限等）
//	配置 0                → 同上（0 视为「用默认」，避免把「漏配」误当「永不超时」）
//	配置 < 0              → 负数，调用方解释为「不设超时」（显式关闭）
//	配置 > 0              → 原值
func (c *Config) PgReadTimeoutMs() int64 {
	if c == nil || c.Feature == nil || c.Feature.PgReadTimeoutMs == nil {
		return PgReadDefaultTimeoutMs
	}
	v := *c.Feature.PgReadTimeoutMs
	if v == 0 {
		return PgReadDefaultTimeoutMs
	}
	return v
}

// Metrics filscan-metrics 的可选配置（全部可省略）。
type Metrics struct {
	Address         *string  `toml:"address"`           // /metrics 监听地址，如 "127.0.0.1:10020"
	Interval        *int64   `toml:"interval"`          // 采集/渲染缓存间隔（秒）
	Tables          []string `toml:"tables"`            // 关键表新鲜度清单，形如 ["chain.actor_actions:epoch"]
	LegacyDelayName *bool    `toml:"legacy_delay_name"` // 是否输出兼容旧规则的 filscan_syncer_delay_height（同口径同值）
	QueryTimeout    *int64   `toml:"query_timeout"`     // 单条 SQL / 链头接口超时（秒）
}

// MetricsAddress 返回 /metrics 监听地址，未配置时返回 fallback。
func (c *Config) MetricsAddress(fallback string) string {
	if c == nil || c.Metrics == nil || c.Metrics.Address == nil || *c.Metrics.Address == "" {
		return fallback
	}
	return *c.Metrics.Address
}

// MetricsIntervalSeconds 返回采集间隔（秒），未配置时返回 0（由调用方兜底）。
func (c *Config) MetricsIntervalSeconds() int64 {
	if c == nil || c.Metrics == nil || c.Metrics.Interval == nil || *c.Metrics.Interval <= 0 {
		return 0
	}
	return *c.Metrics.Interval
}

// MetricsQueryTimeoutSeconds 返回单查询超时（秒），未配置时返回 0（由调用方兜底）。
func (c *Config) MetricsQueryTimeoutSeconds() int64 {
	if c == nil || c.Metrics == nil || c.Metrics.QueryTimeout == nil || *c.Metrics.QueryTimeout <= 0 {
		return 0
	}
	return *c.Metrics.QueryTimeout
}

// MetricsTables 返回配置的关键表清单；未配置时返回 nil（由调用方使用内置清单）。
func (c *Config) MetricsTables() []string {
	if c == nil || c.Metrics == nil {
		return nil
	}
	return c.Metrics.Tables
}

// MetricsLegacyDelayName 返回是否输出兼容旧规则的旧指标名；未配置时返回 nil。
func (c *Config) MetricsLegacyDelayName() *bool {
	if c == nil || c.Metrics == nil {
		return nil
	}
	return c.Metrics.LegacyDelayName
}

type Londobell struct {
	AggAddress      *string `toml:"agg_address"`
	AdapterAddress  *string `toml:"adapter_address"`
	MinerAggAddress *string `toml:"miner_agg_address"`
}

type Solidity struct {
	SolcPath          *string `toml:"solc_path"`           // solc绝对路径
	SolcSelectPath    *string `toml:"solc_select_path"`    // solc-select绝对路径
	ContractDirectory *string `toml:"contracts_directory"` // 合约文件夹
}

type Redis struct {
	RedisAddress *string `toml:"redis_address"` // redis 地址
	MaxIdle      *int    `toml:"max_idle"`
	MaxActive    *int    `toml:"max_active"`
	IdleTimeout  *int64  `toml:"idle_timeout"`
}

type Syncer struct {
	EnableSyncers   []string `toml:"enable_syncers"` // 开启的同步器列表
	EpochsChunk     *int64   `toml:"epochs_chunk"`
	EpochsThreshold *int64   `toml:"epochs_threshold"`
	// DataErrorThreshold 数据级错误（聚合器业务码 code:1、响应体解码失败等不可恢复错误）
	// 连续失败多少次后，登记并跳过该高度继续往下同步；传输级错误（网络/超时）不受此配置影响，
	// 始终按原有逻辑重试。未配置或 <=0 时取同步器内置默认值 5。
	DataErrorThreshold *int64 `toml:"data_error_threshold"`
	// UnrecoverableErrorThreshold 节点侧状态不可用（不可恢复）错误（日志形如 load state tree /
	// failed to load hamt node / ipld: could not find —— 本地节点只保留近期状态窗口，历史状态已被裁掉）
	// 连续失败多少次后登记并跳过；该计数与 DataErrorThreshold **独立**。
	// 未配置或 <=0 时取同步器内置默认值 20（重试间隔默认 15s ⇒ 约 5 分钟）。
	UnrecoverableErrorThreshold *int64 `toml:"unrecoverable_error_threshold"`
	// StateGapJumpMinGap 触发「一次跳过整段不可恢复区间」的最小缺口（链头 − 当前高度）。
	// 实测这类缺口是几万个连续高度，一个高度一个高度跳过没有意义；缺口 >= 该值时一次跳到
	// 链头 − StateGapJumpMargin。未配置或 <=0 时取同步器内置默认值 1000。
	StateGapJumpMinGap *int64 `toml:"state_gap_jump_min_gap"`
	// StateGapJumpMargin 区间跳的目标高度 = 链头 − margin（留出安全边界，避免贴着链头被回滚）。
	// 未配置或 <=0 时取同步器内置默认值 200。
	StateGapJumpMargin *int64 `toml:"state_gap_jump_margin"`
	// SyncLargeTransfers 是否开启「按高度把大额转账增量写进 PostgreSQL 表 chain.large_transfers」
	// 这一步（默认 false）：关闭时同步器不会注册该任务，一个 SQL 都不发、行为与打补丁前完全一致；
	// 开启后由 chain 同步器在**已有的** traces 获取链路上顺带维护（不新增聚合器请求）。
	// 表结构见 migration/35.large_transfers.sql（在另一条工作线上）；写入口径见
	// modules/syncer/chain/large-transfer-task/README.md。
	SyncLargeTransfers *bool `toml:"sync_large_transfers"`
}

// SyncLargeTransfersValue 返回是否开启「大额转账增量维护」；未配置时返回 false（默认关闭）。
// 与 Syncer 段其它开关一样用方法而不是直接解引用指针：老配置文件里没有该字段时不应 panic。
func (s *Syncer) SyncLargeTransfersValue() bool {
	if s == nil || s.SyncLargeTransfers == nil {
		return false
	}
	return *s.SyncLargeTransfers
}

// DataErrorThresholdValue 返回配置的「数据级错误跳过阈值」；未配置时返回 0（由同步器回退到内置默认值 5）。
// 用方法而不是直接解引用指针：老配置文件里没有该字段时不应 panic。
func (s *Syncer) DataErrorThresholdValue() int64 {
	if s == nil || s.DataErrorThreshold == nil {
		return 0
	}
	return *s.DataErrorThreshold
}

// UnrecoverableErrorThresholdValue 返回配置的「节点侧不可恢复错误跳过阈值」；
// 未配置时返回 0（由同步器回退到内置默认值 20）。同样避免老配置文件缺字段时 panic。
func (s *Syncer) UnrecoverableErrorThresholdValue() int64 {
	if s == nil || s.UnrecoverableErrorThreshold == nil {
		return 0
	}
	return *s.UnrecoverableErrorThreshold
}

// StateGapJumpMinGapValue 返回配置的「区间跳最小缺口」；未配置时返回 0（由同步器回退到内置默认值 1000）。
func (s *Syncer) StateGapJumpMinGapValue() int64 {
	if s == nil || s.StateGapJumpMinGap == nil {
		return 0
	}
	return *s.StateGapJumpMinGap
}

// StateGapJumpMarginValue 返回配置的「区间跳目标 margin」；未配置时返回 0（由同步器回退到内置默认值 200）。
func (s *Syncer) StateGapJumpMarginValue() int64 {
	if s == nil || s.StateGapJumpMargin == nil {
		return 0
	}
	return *s.StateGapJumpMargin
}

type Mail struct {
	Client   *string `toml:"client"`
	Port     *int    `toml:"port"`
	Username *string `toml:"username"`
	Password *string `toml:"password"`
}

type ALi struct {
	MsgClient       *string `toml:"msg_client"`
	CallClient      *string `toml:"call_client"`
	MsgTemplateCode *string `toml:"msg_template_code"`
	TtsCode         *string `toml:"tts_code"`
	AccessKeyId     *string `toml:"access_key_id"`
	AccessKeySecret *string `toml:"access_key_secret"`
}

type Pro struct {
	JwtSecret          string `toml:"jwt_secret"`
	DisableSyncFund    bool   `toml:"disable_sync_fund"`
	DisableSyncInfo    bool   `toml:"disable_sync_info"`
	DisableSyncSector  bool   `toml:"disable_sync_sector"`
	DisableSyncBalance bool   `toml:"disable_sync_balance"`
	VipSecret          string `toml:"vip_secret"`
}
