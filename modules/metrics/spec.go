package metrics

import (
	"fmt"
	"regexp"
	"strings"
)

// 采集目标（表名 / 列名）的白名单字符集。
//
// 为什么必须严格校验：PostgreSQL 不支持把「标识符」作为绑定参数传给驱动，
// 表名/列名只能拼进 SQL 文本。因此这里用白名单正则把注入面彻底关掉：
// 不合法的规格在 ParseTableSpec 阶段就被拒绝（配置写错 ⇒ 启动即报错，而不是
// 把一个可疑字符串送进数据库）。
var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// DefaultColumn 规格未显式给出高度列时使用的列名。
const DefaultColumn = "epoch"

// TableSpec 一条「关键表新鲜度」采集目标。
//
// 语义：Table.Column 是该表数据高度的单调游标，表新鲜度 = 链头高度 − max(Table.Column)。
// 例：chain.actor_actions 的 epoch 列、fevm.evm_transfers 的 epoch 列。
type TableSpec struct {
	Schema string
	Table  string
	Column string
}

// Qualified 返回 "schema.table"（用于指标标签与日志）。
func (t TableSpec) Qualified() string {
	return t.Schema + "." + t.Table
}

// String 返回规格原文 "schema.table:column"（用于日志）。
func (t TableSpec) String() string {
	return t.Qualified() + ":" + t.Column
}

// Validate 校验标识符合法性（防注入的最后一道）。
func (t TableSpec) Validate() error {
	if !identRe.MatchString(t.Schema) {
		return fmt.Errorf("非法 schema 名: %q", t.Schema)
	}
	if !identRe.MatchString(t.Table) {
		return fmt.Errorf("非法表名: %q", t.Table)
	}
	if !identRe.MatchString(t.Column) {
		return fmt.Errorf("非法列名: %q", t.Column)
	}
	return nil
}

// MaxSQL 生成该表的高度最大值查询。
//
// 生成形态（标识符一律加双引号以固定大小写）：
//
//	select max("epoch") from "chain"."actor_actions"
//
// 表级错误（表不存在、权限不足、超时）由上层容错：只累加采集错误计数，
// 不中断整轮采集——监控系统不能成为压垮业务库的那根稻草，也不能因为
// 一张表缺失就整轮没有数据。
func (t TableSpec) MaxSQL() string {
	return fmt.Sprintf(`select max("%s") from "%s"."%s"`, t.Column, t.Schema, t.Table)
}

// QuoteQualified 把 "schema.table" 拆开后逐段加引号（供游标表等固定查询复用）。
func QuoteQualified(qualified string) (string, error) {
	parts := strings.Split(qualified, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("表名必须是 schema.table 形式: %q", qualified)
	}
	for _, p := range parts {
		if !identRe.MatchString(p) {
			return "", fmt.Errorf("非法表名: %q", qualified)
		}
	}
	return fmt.Sprintf(`"%s"."%s"`, parts[0], parts[1]), nil
}

// ParseTableSpec 解析形如 "chain.actor_actions:epoch" 的规格。
//
// 规则：
//   - 必须带 schema（"chain.actor_actions"），不接受 search_path 兜底；
//   - 高度列可省略，省略时取 DefaultColumn（"epoch"）；
//   - schema / 表 / 列三段都必须匹配 ^[A-Za-z_][A-Za-z0-9_]*$；
//   - 统一归一化为小写（本库全部是小写表名；SQL 里标识符一律加双引号，
//     而归一化后就不存在「写了大写、实际查不到」的隐式陷阱）。
func ParseTableSpec(s string) (TableSpec, error) {
	var spec TableSpec
	raw := strings.TrimSpace(s)
	if raw == "" {
		return spec, fmt.Errorf("空表规格")
	}
	tablePart := raw
	column := DefaultColumn
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		tablePart = strings.TrimSpace(raw[:i])
		column = strings.TrimSpace(raw[i+1:])
		if column == "" {
			return spec, fmt.Errorf("表规格 %q 的高度列为空", raw)
		}
	}
	parts := strings.Split(tablePart, ".")
	if len(parts) != 2 {
		return spec, fmt.Errorf("表规格 %q 必须是 schema.table[:column] 形式", raw)
	}
	spec = TableSpec{
		Schema: strings.ToLower(parts[0]),
		Table:  strings.ToLower(parts[1]),
		Column: strings.ToLower(column),
	}
	if err := spec.Validate(); err != nil {
		return TableSpec{}, fmt.Errorf("表规格 %q 不合法: %w", raw, err)
	}
	return spec, nil
}

// defaultTableSpecStrings 内置关键表清单（可被配置 [metrics].tables 整体替换）。
//
// 选取原则：每一条对应一个主同步器的关键产物，任一同步器停摆 ⇒ 至少一条
// table lag 变大。清单刻意取「小而不空」的表/索引扫描便宜的 max（都按高度分区
// 且带 (epoch, ...) 索引），单轮采集成本可控。
//
// 事故对照（2026-09 主网索引链静默停摆 18 天）：当时 chain/miner 两条游标只走到
// 6357057，而链头已到 6408450；只要 chain.sync_syncers 在清单里，这张表就会
// 停在 6357057 并暴露 51393 的落后量。
var defaultTableSpecStrings = []string{
	"chain.sync_syncers:epoch",       // 同步器游标表（任一同步器停摆立即显形，最关键）
	"chain.sync_syncer_epochs:epoch", // 同步器逐高度执行留痕（含空高度，用于区分「没跑」和「跑了但没数据」）
	"chain.actor_actions:epoch",      // actor syncer
	"chain.rich_actors:epoch",        // chain syncer（账户余额）
	"chain.miner_infos:epoch",        // miner syncer
	"pro.miner_sectors:epoch",        // sector syncer
	"fevm.evm_transfers:epoch",       // evm syncer
	"fevm.erc_20_transfers:epoch",    // erc20 syncer
	"fns.transfers:epoch",            // fns syncer
}

// DefaultTableSpecs 返回内置关键表清单的副本（调用方可自由增删）。
func DefaultTableSpecs() []TableSpec {
	specs := make([]TableSpec, 0, len(defaultTableSpecStrings))
	for _, s := range defaultTableSpecStrings {
		spec, err := ParseTableSpec(s)
		if err != nil {
			// 内置清单写错属于编码期错误，直接 panic 暴露（不会在生产配置下触发）。
			panic(fmt.Errorf("内置关键表清单不合法 %q: %w", s, err))
		}
		specs = append(specs, spec)
	}
	return specs
}

// ParseTableSpecs 批量解析配置里的表清单；任一不合法即返回错误（让配置错误在启动时暴露）。
func ParseTableSpecs(list []string) ([]TableSpec, error) {
	specs := make([]TableSpec, 0, len(list))
	for _, s := range list {
		spec, err := ParseTableSpec(s)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// NetworkName 返回网络标签取值（用于所有指标的 network 标签）。
func NetworkName(testNet bool) string {
	if testNet {
		return "calibnet"
	}
	return "mainnet"
}
