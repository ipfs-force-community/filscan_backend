package offlinereplay

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// 本文件是「按高度清单跑」（--epochs-file）的解析与计划装配：
//
//	① EpochList —— 高度清单的解析结果（升序去重 + 输入侧统计）；
//	② Plan      —— 回放计划：连续区间（--start/--end）与高度清单（--epochs-file）二选一。
//
// 为什么需要清单模式：缺口常常是**离散高度**而不是连续区间（实测 chain.actor_actions 在
// [6280000, 6412000] 内掉的 909 段 / 1333 个高度就是离散的）。按区间重跑会把整段十几万高度
// 一起重算，代价不可接受；清单模式只跑这些高度。
//
// 清单内部仍按**升序逐个高度**处理：这些高度与链上状态无关（各高度独立计算），
// 固定升序只是让同样的输入产生同样的处理顺序与同序的报告（可重复、可对照）。

// EpochList 显式高度清单（--epochs-file 的内容）。
type EpochList struct {
	source     string  // 来源（文件路径），只用于报错与报告
	epochs     []int64 // 升序去重后的高度
	lines      int     // 输入总行数（含空行/注释行）
	skipped    int     // 被跳过的空行/注释行数
	duplicates int     // 被合并的重复高度个数
}

// ParseEpochList 解析高度清单文本（LoadEpochList 读文件后调用；单测直接喂文本）。
//
// 规则：
//   - 一行一个高度；行首行尾空白忽略；
//   - 空行跳过；`#` 起头、或行尾 `#` 之后的内容按注释跳过；
//   - 高度必须是 > 0 的十进制整数，否则报错并指出行号与原文行；
//   - 重复高度合并（同一高度只跑一次）；乱序输入按升序排列。
//
// 解析后一个高度都不剩（空文件 / 全是空行或注释）⇒ 报错：
// 空清单会把「什么都没跑」伪装成「跑完了」，必须挡住。
func ParseEpochList(text, source string) (EpochList, error) {
	out := EpochList{source: source}
	seen := make(map[int64]struct{})

	// 末尾换行不算「多一行」：让 Lines()/Skipped() 与人的数法一致（否则每份正常清单都会多记 1 行）
	text = strings.TrimSuffix(text, "\n")

	for i, raw := range strings.Split(text, "\n") {
		lineNo := i + 1
		out.lines++

		line := strings.TrimSpace(raw)
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			out.skipped++
			continue
		}

		v, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			return EpochList{}, fmt.Errorf("%s 第 %d 行不是合法高度: %q", source, lineNo, strings.TrimSpace(raw))
		}
		if v <= 0 {
			return EpochList{}, fmt.Errorf("%s 第 %d 行高度必须 > 0, 收到: %d", source, lineNo, v)
		}
		if _, ok := seen[v]; ok {
			out.duplicates++
			continue
		}
		seen[v] = struct{}{}
		out.epochs = append(out.epochs, v)
	}

	if len(out.epochs) == 0 {
		return EpochList{}, fmt.Errorf("%s 没有解析出任何高度（空文件，或全是空行/注释行）: 拒绝以空清单运行", source)
	}

	sort.Slice(out.epochs, func(i, j int) bool { return out.epochs[i] < out.epochs[j] })
	return out, nil
}

// LoadEpochList 读取并解析高度清单文件
func LoadEpochList(path string) (EpochList, error) {
	if strings.TrimSpace(path) == "" {
		return EpochList{}, fmt.Errorf("高度清单文件路径为空")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return EpochList{}, fmt.Errorf("读取高度清单 %s 失败: %w", path, err)
	}
	return ParseEpochList(string(data), path)
}

// Epochs 升序去重后的高度（返回副本，调用方改不动内部状态）
func (l EpochList) Epochs() []int64 { return append([]int64(nil), l.epochs...) }

// Count 清单里的高度个数
func (l EpochList) Count() int { return len(l.epochs) }

// Source 清单来源（文件路径）
func (l EpochList) Source() string { return l.source }

// Min 最小高度（Count() > 0 时有效）
func (l EpochList) Min() int64 { return l.epochs[0] }

// Max 最大高度（Count() > 0 时有效）
func (l EpochList) Max() int64 { return l.epochs[len(l.epochs)-1] }

// Lines 输入总行数
func (l EpochList) Lines() int { return l.lines }

// Skipped 被跳过的空行/注释行数
func (l EpochList) Skipped() int { return l.skipped }

// Duplicates 被合并的重复高度个数
func (l EpochList) Duplicates() int { return l.duplicates }

func (l EpochList) String() string {
	if l.Count() == 0 {
		return fmt.Sprintf("%s: 高度清单为空", l.source)
	}
	s := fmt.Sprintf("%s: %d 个高度（升序去重后），跨 [%d, %d]；输入 %d 行：跳过空行/注释 %d 行",
		l.source, l.Count(), l.Min(), l.Max(), l.lines, l.skipped)
	if l.duplicates > 0 {
		s += fmt.Sprintf("，合并重复高度 %d 个", l.duplicates)
	}
	return s + ")"
}

// Plan 一次离线回放的计划：连续区间（--start/--end）与显式高度清单（--epochs-file）二选一。
type Plan struct {
	// Range 连续区间模式（IsList() == false 时有效）
	Range Range
	// List 高度清单模式（IsList() == true 时有效，Count() > 0）
	List EpochList

	list bool
}

// ResolvePlan 解析并校验回放计划 —— 「区间 / 清单」的互斥判定只有这一处（命令层不再各自判断）。
//
//   - 给出 --epochs-file：禁止同时给 --start/--end（两者语义冲突，猜哪个都不是「不猜」）；
//   - 未给 --epochs-file：必须给出 --start/--end（同时给出 —— 只给一半会被下面的校验按单边非法拒绝）；
//   - 清单文件读不到 / 解析不出任何高度 ⇒ 报错（不静默降级成空跑）。
func ResolvePlan(start, end int64, epochsFile string) (Plan, error) {
	if strings.TrimSpace(epochsFile) != "" {
		if start != 0 || end != 0 {
			return Plan{}, fmt.Errorf("--epochs-file 与 --start/--end 互斥：已给出高度清单 %s，"+
				"请去掉 --start/--end（或改用连续区间模式）", epochsFile)
		}
		list, err := LoadEpochList(epochsFile)
		if err != nil {
			return Plan{}, err
		}
		return Plan{List: list, list: true}, nil
	}

	if start == 0 && end == 0 {
		return Plan{}, fmt.Errorf("未给出高度范围: 请给出 --start/--end（连续区间）或 --epochs-file（显式高度清单）")
	}
	rng, err := ResolveRange(start, end)
	if err != nil {
		return Plan{}, err
	}
	return Plan{Range: rng}, nil
}

// IsList 是否按高度清单跑（false = 连续区间）
func (p Plan) IsList() bool { return p.list }

// Count 本次要跑的高度个数
func (p Plan) Count() int64 {
	if p.list {
		return int64(p.List.Count())
	}
	return p.Range.Count()
}

func (p Plan) String() string {
	if p.list {
		return p.List.String()
	}
	if p.Range.Count() <= 0 {
		return "未给出高度区间"
	}
	return p.Range.String()
}
