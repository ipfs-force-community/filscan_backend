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
