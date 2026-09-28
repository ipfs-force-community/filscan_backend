// Package metrics 产出「同步落后高度」与「关键表新鲜度」指标。
//
// 为什么需要这个包（事故背景）：2026-09 主网索引链静默停摆 18 天而无有效告警。
// 现役告警依赖的 filscan_syncer_delay_height 口径坏（实测 chain=2 / miner=60，
// 真实落后 51393 个高度），且四条规则引用的指标在 Prometheus 里不存在。
// 本包给监控侧一个**口径明确、可判、单点失败不互相拖累**的数据源：
//
//	filscan_syncer_lag_height{syncer}   = 链头高度 − 该同步器已处理高度
//	                                      （chain.sync_syncers.epoch，单位：高度/epoch）
//	filscan_table_lag_height{table}     = 链头高度 − 该表 max(高度列)
//	                                      （单位：高度/epoch，直接回答「哪张表停在多少高度」）
//
// 设计约束（与生产环境对齐）：
//   - 不引入任何第三方依赖：只做「取数 + Prometheus 文本格式渲染」，用标准库完成；
//   - 容错：某张表不存在 / 查询超时 / 链头接口失败，只累加 *_collect_errors_total，
//     其余指标照常产出，整轮采集绝不因单点失败而中断；
//   - 采集频率低、单查询带超时：监控不得把业务库拖慢。
package metrics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
)

// Type Prometheus 指标类型（本包只用到 gauge / counter）。
type Type string

const (
	Gauge   Type = "gauge"
	Counter Type = "counter"
)

// Sample 一条样本（一个标签组合 + 一个值）。
type Sample struct {
	Labels map[string]string
	Value  float64
}

// Metric 一个指标族（同名、同 HELP、同 TYPE）。
type Metric struct {
	Name    string
	Help    string
	Type    Type
	Samples []Sample
}

// Set 一次采集产出的全部指标族，按加入顺序渲染。
type Set struct {
	metrics []*Metric
	index   map[string]*Metric
}

// NewSet 新建空集合。
func NewSet() *Set {
	return &Set{index: make(map[string]*Metric)}
}

// Add 追加一条样本。同名指标的 HELP/TYPE 必须一致，否则返回错误。
func (s *Set) Add(name, help string, typ Type, labels map[string]string, value float64) error {
	m, ok := s.index[name]
	if !ok {
		m = &Metric{Name: name, Help: help, Type: typ}
		s.index[name] = m
		s.metrics = append(s.metrics, m)
	} else if m.Help != help || m.Type != typ {
		return fmt.Errorf("指标 %s 的 HELP/TYPE 必须一致（已有 help=%q type=%q）", name, m.Help, m.Type)
	}
	m.Samples = append(m.Samples, Sample{Labels: labels, Value: value})
	return nil
}

// Render 按 Prometheus 文本格式写出：每个指标族先 HELP 再 TYPE，再逐条样本。
// 样本按标签排序，保证同一份数据每次渲染字节一致（便于 diff 与测试）。
func (s *Set) Render(w io.Writer) error {
	for _, m := range s.metrics {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n", m.Name, escapeHelp(m.Help)); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "# TYPE %s %s\n", m.Name, m.Type); err != nil {
			return err
		}
		samples := make([]Sample, len(m.Samples))
		copy(samples, m.Samples)
		sort.SliceStable(samples, func(i, j int) bool {
			return labelKey(samples[i].Labels) < labelKey(samples[j].Labels)
		})
		seen := make(map[string]struct{}, len(samples))
		for _, smp := range samples {
			key := labelKey(smp.Labels)
			if _, dup := seen[key]; dup {
				// 同一指标名下重复的标签组合会让 Prometheus 整轮抓取失败
				// （"duplicate sample for timestamp"），这里直接去重丢弃后一条。
				continue
			}
			seen[key] = struct{}{}
			if _, err := fmt.Fprintf(w, "%s%s %s\n", m.Name, renderLabels(smp.Labels), formatValue(smp.Value)); err != nil {
				return err
			}
		}
	}
	return nil
}

// Bytes 渲染为字节切片（供 HTTP 响应与 --once 直接输出）。
func (s *Set) Bytes() []byte {
	var b strings.Builder
	b.Grow(1024)
	if err := s.Render(&b); err != nil {
		return nil
	}
	return []byte(b.String())
}

func labelKey(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(labels[k])
	}
	return b.String()
}

func renderLabels(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(k)
		b.WriteString(`="`)
		b.WriteString(escapeLabelValue(labels[k]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func escapeHelp(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

func escapeLabelValue(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return strings.ReplaceAll(s, "\n", `\n`)
}

// formatValue 按 Prometheus 文本格式输出数值：整数不带小数点（避免出现 6.4e+06 这种写法）。
func formatValue(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	case v == math.Trunc(v) && math.Abs(v) < 1e15:
		return strconv.FormatInt(int64(v), 10)
	default:
		return strconv.FormatFloat(v, 'g', -1, 64)
	}
}
