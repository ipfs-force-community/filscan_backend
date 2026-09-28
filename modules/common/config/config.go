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
}

// DataErrorThresholdValue 返回配置的「数据级错误跳过阈值」；未配置时返回 0（由同步器回退到内置默认值 5）。
// 用方法而不是直接解引用指针：老配置文件里没有该字段时不应 panic。
func (s *Syncer) DataErrorThresholdValue() int64 {
	if s == nil || s.DataErrorThreshold == nil {
		return 0
	}
	return *s.DataErrorThreshold
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
