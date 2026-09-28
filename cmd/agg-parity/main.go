// filscan-agg-parity：三个统计端点「聚合器 vs PG」两路径一致性校验（上线前通关条件）。
//
// 用法（跑在 backend 主机上，用与 filscan-api 同一份 config.toml）：
//
//	# 1) 单矿工 + 区间：三个端点全比（wincount 会按 miner 过滤后比）
//	filscan-agg-parity -c /etc/filscan/config.toml -miner f01234 -start 4300000 -end 4300100
//
//	# 2) 只比某个端点；不指定 -miner 时 miners_blockreward / wincount 比**全矿工**（行多，区间选小一点）
//	filscan-agg-parity -c /etc/filscan/config.toml -start 4300000 -end 4300010 -endpoints miners_blockreward,wincount
//
//	# 3) 只比单矿工出块奖励，且输出 JSON（供上线 checklist / 工单留痕）
//	filscan-agg-parity -c /etc/filscan/config.toml -miner f01234 -start 4300000 -end 4300100 -endpoints miner_blockreward -json
//
// 退出码：0 = 全部端点 PASS；1 = 存在差异（数值不等 / 点位缺失 / 举例：金额文本形态不同且未关 strict）
//
//	2 = 参数或初始化错误（配置/连库/聚合器地址）。
//
// 口径与字段映射见 modules/common/infra/dal/dal_biz_miner_reward_range.go 与
// modules/filscan/biz/browser/agg_pg_reward.go 的文件头注释；比对逻辑在 modules/filscan/service/rewardparity。
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/injector"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/rewardparity"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_config"
	"gorm.io/gorm"
)

var (
	// Name 编译期注入的进程名。
	Name = "filscan-agg-parity"
	// Version 编译期注入的版本号（Makefile -ldflags）。
	Version string
)

type options struct {
	configFile   string
	miner        string
	start        int64
	end          int64
	endpoints    string
	maxExamples  int
	strictFormat bool
	jsonOut      bool
	diagnose     bool
	timeout      time.Duration
	showSQL      bool
	showVersion  bool
}

func main() {
	os.Exit(run())
}

func run() int {
	var opt options
	fs := flag.NewFlagSet(Name, flag.ExitOnError)
	fs.StringVar(&opt.configFile, "c", "", "配置文件路径（与 filscan-api / filscan-syncer 同一份 config.toml），必填")
	fs.StringVar(&opt.miner, "miner", "", "矿工地址（f01234 或 01234 均可）；给定后三个端点都按该矿工过滤比对")
	fs.Int64Var(&opt.start, "start", -1, "起始高度（含），必填")
	fs.Int64Var(&opt.end, "end", -1, "结束高度（**不含**，与聚合器管线 Epoch<$lt 一致），必填")
	fs.StringVar(&opt.endpoints, "endpoints", strings.Join(rewardparity.AllEndpoints(), ","),
		"要比对的端点，逗号分隔：miner_blockreward,miners_blockreward,wincount")
	fs.IntVar(&opt.maxExamples, "max-examples", 10, "每类差异最多打印多少条样例（计数仍是全量）")
	fs.BoolVar(&opt.strictFormat, "strict-format", true,
		"金额文本形态是否也算失败：true（默认）= 只要聚合器与 PG 的 decimal 文本不同就判 FAIL")
	fs.BoolVar(&opt.jsonOut, "json", false, "以 JSON 输出（便于留痕/机检）")
	fs.BoolVar(&opt.diagnose, "diag", true, "出现差异时额外打印只读诊断（覆盖范围/重复行/地址形态）")
	fs.DurationVar(&opt.timeout, "timeout", 0, "单次取数超时；<=0 取配置 [feature].pg_read_timeout_ms（默认 5s）")
	fs.BoolVar(&opt.showSQL, "print-sql", false, "打印 PG 读路径使用的 SQL（用于与聚合器管线逐条对照）")
	fs.BoolVar(&opt.showVersion, "version", false, "打印版本后退出")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s %s\n\n用法：\n", Name, Version)
		fs.PrintDefaults()
	}
	_ = fs.Parse(os.Args[1:])

	if opt.showVersion {
		fmt.Printf("%s %s\n", Name, Version)
		return 0
	}
	if opt.configFile == "" {
		fmt.Fprintln(os.Stderr, "缺少 -c 配置文件路径")
		fs.Usage()
		return 2
	}
	if opt.start < 0 || opt.end < 0 || opt.end <= opt.start {
		fmt.Fprintln(os.Stderr, "区间非法：需要 0 <= start < end（区间语义为左闭右开 [start, end)）")
		return 2
	}

	eps, err := parseEndpoints(opt.endpoints)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}

	conf := &config.Config{}
	if err := _config.UnmarshalConfigFile(opt.configFile, conf); err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败 %s: %s\n", opt.configFile, err)
		return 2
	}
	if conf.DSN == nil || *conf.DSN == "" {
		fmt.Fprintln(os.Stderr, "配置缺少 DSN（依赖数据库），无法读 PG")
		return 2
	}
	if conf.Londobell == nil || conf.Londobell.AggAddress == nil || *conf.Londobell.AggAddress == "" {
		fmt.Fprintln(os.Stderr, "配置缺少 [londobell].agg_address，无法读聚合器")
		return 2
	}

	timeout := opt.timeout
	if timeout <= 0 {
		timeout = time.Duration(conf.PgReadTimeoutMs()) * time.Millisecond
	}

	db, cleanup, err := injector.NewGormDB(conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接 PG 失败: %s\n", err)
		return 2
	}
	defer cleanup()

	agg, err := injector.NewLondobellAgg(conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化聚合器客户端失败: %s\n", err)
		return 2
	}

	var miner chain.SmartAddress
	if opt.miner != "" {
		miner = chain.SmartAddress(opt.miner)
		if err := miner.Valid(); err != nil {
			fmt.Fprintf(os.Stderr, "-miner %q 不是合法地址: %s\n", opt.miner, err)
			return 2
		}
	}

	params := rewardparity.Params{Miner: miner, Start: chain.Epoch(opt.start), End: chain.Epoch(opt.end)}
	reader := dal.NewMinerRewardRangeDal(db)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if !opt.jsonOut {
		fmt.Printf("%s %s\n", Name, Version)
		fmt.Printf("聚合器: %s\n", *conf.Londobell.AggAddress)
		fmt.Printf("区间:   [%d, %d) 左闭右开（聚合器管线条件是 Epoch>=$gte && Epoch<$lt）；矿工: %s；超时: %s\n",
			opt.start, opt.end, emptyAs(miner.Address(), "<全矿工>"), timeout)
		fmt.Println("口径:   PG 侧先 DISTINCT ON(epoch, miner) 去重再聚合；miner 列在库内是**带前缀**的 f0… 形态")
		fmt.Println("取数:   聚合器侧**始终直连**（绕过 API 读路径的 [feature] 开关）⇒ 结论与开关当前状态无关")
		if opt.showSQL {
			fmt.Println("PG 读路径 SQL:")
			for _, s := range []string{dal.SQLMinerBlockRewardRange, dal.SQLMinersBlockRewardRange, dal.SQLMinerWinCountsRange} {
				fmt.Println(strings.TrimSpace(s))
				fmt.Println("---")
			}
		}
	}

	results := make([]rewardparity.Result, 0, len(eps))
	for _, ep := range eps {
		r := compareEndpoint(ctx, ep, params, agg, reader, opt.maxExamples)
		results = append(results, r)
	}

	if opt.jsonOut {
		return emitJSON(results, opt.strictFormat)
	}
	return emitText(results, opt, db, params)
}

func compareEndpoint(ctx context.Context, ep string, p rewardparity.Params, agg londobell.Agg, reader repository.MinerRewardRange, maxExamples int) rewardparity.Result {
	maxEx := rewardparity.MaxExamples(maxExamples)

	switch ep {
	case rewardparity.EndpointMinerBlockReward:
		// 聚合器侧：区间 + 矿工（Index/Limit 与线上管线一样不参与过滤）
		start, end := p.Start, p.End
		var aggRows []*londobell.MinerBlockReward
		var aggErr error
		aggStart := time.Now()
		if p.Miner != "" {
			aggRows, aggErr = agg.MinerBlockReward(ctx, p.Miner, types.Filters{Start: &start, End: &end})
		} else {
			aggErr = fmt.Errorf("miner_blockreward 端点必须给 -miner（该端点本身按矿工查）")
		}
		aggLatency := time.Since(aggStart)

		var pgRows []*bo.MinerEpochReward
		var pgErr error
		pgStart := time.Now()
		if p.Miner != "" {
			pgRows, pgErr = reader.MinerBlockRewardRange(ctx, p.Miner.Address(), p.Start, p.End)
		}
		pgLatency := time.Since(pgStart)

		r := rewardparity.CompareMinerBlockReward(p, aggRows, pgRows, maxEx)
		r.AggErr, r.PgErr = aggErr, pgErr
		r.AggLatency, r.PgLatency = aggLatency, pgLatency
		return r

	case rewardparity.EndpointMinersBlockReward:
		aggStart := time.Now()
		aggRows, aggErr := agg.MinersBlockReward(ctx, p.Start, p.End)
		aggLatency := time.Since(aggStart)

		pgStart := time.Now()
		pgRows, pgErr := reader.MinersBlockRewardRange(ctx, p.Start, p.End)
		pgLatency := time.Since(pgStart)

		aggRows, pgRows = filterMiners(p.Miner, aggRows, pgRows)

		r := rewardparity.CompareMinersBlockReward(p, aggRows, pgRows, maxEx)
		r.AggErr, r.PgErr = aggErr, pgErr
		r.AggLatency, r.PgLatency = aggLatency, pgLatency
		return r

	case rewardparity.EndpointWinCount:
		aggStart := time.Now()
		aggRows, aggErr := agg.WinCount(ctx, p.Start, p.End)
		aggLatency := time.Since(aggStart)

		pgStart := time.Now()
		pgRows, pgErr := reader.MinerWinCountsRange(ctx, p.Start, p.End)
		pgLatency := time.Since(pgStart)

		aggRows, pgRows = filterWinCounts(p.Miner, aggRows, pgRows)

		r := rewardparity.CompareWinCount(p, aggRows, pgRows, maxEx)
		r.AggErr, r.PgErr = aggErr, pgErr
		r.AggLatency, r.PgLatency = aggLatency, pgLatency
		return r
	}

	return rewardparity.Result{Endpoint: ep, Params: p, AggErr: fmt.Errorf("未知端点 %q", ep)}
}

// filterMiners 按矿工过滤两侧（只用于「聚焦某矿工」的比对，不会放宽口径）。
func filterMiners(miner chain.SmartAddress, aggRows []*londobell.MinersBlockReward, pgRows []*bo.MinerEpochReward) ([]*londobell.MinersBlockReward, []*bo.MinerEpochReward) {
	if miner == "" {
		return aggRows, pgRows
	}
	target := miner.Address()
	var aggOut []*londobell.MinersBlockReward
	for _, row := range aggRows {
		if row != nil && chain.SmartAddress(row.Id.Miner).Address() == target {
			aggOut = append(aggOut, row)
		}
	}
	var pgOut []*bo.MinerEpochReward
	for _, row := range pgRows {
		if row != nil && chain.SmartAddress(row.Miner).Address() == target {
			pgOut = append(pgOut, row)
		}
	}
	return aggOut, pgOut
}

// filterWinCounts 按矿工过滤两侧。
func filterWinCounts(miner chain.SmartAddress, aggRows []*londobell.MinerWinCount, pgRows []*bo.AccWinCount) ([]*londobell.MinerWinCount, []*bo.AccWinCount) {
	if miner == "" {
		return aggRows, pgRows
	}
	target := miner.Address()
	var aggOut []*londobell.MinerWinCount
	for _, row := range aggRows {
		if row != nil && chain.SmartAddress(row.Id).Address() == target {
			aggOut = append(aggOut, row)
		}
	}
	var pgOut []*bo.AccWinCount
	for _, row := range pgRows {
		if row != nil && chain.SmartAddress(row.Miner).Address() == target {
			pgOut = append(pgOut, row)
		}
	}
	return aggOut, pgOut
}

func emitText(results []rewardparity.Result, opt options, db *gorm.DB, p rewardparity.Params) int {
	allPass := true
	for _, r := range results {
		verdict := "PASS"
		if !r.Pass(opt.strictFormat) {
			verdict = "FAIL"
			allPass = false
		}
		fmt.Printf("\n===== %s: %s =====\n", r.Endpoint, verdict)
		if r.AggErr != nil {
			fmt.Printf("聚合器侧错误: %s\n", r.AggErr)
		}
		if r.PgErr != nil {
			fmt.Printf("PG 侧错误:     %s\n", r.PgErr)
		}
		fmt.Printf("行数:   聚合器 %d 行 (%.0fms)  vs  PG %d 行 (%.0fms)\n",
			r.AggRows, float64(r.AggLatency.Microseconds())/1000, r.PgRows, float64(r.PgLatency.Microseconds())/1000)
		fmt.Printf("总量:   聚合器 reward=%s block=%d win=%d gasReward=%s\n",
			r.AggSumReward.String(), r.AggSumBlock, r.AggSumWin, r.AggSumGas.String())
		fmt.Printf("        PG     reward=%s block=%d win=%d gasReward=（PG 无该列，PG 路径恒 0）\n",
			r.PgSumReward.String(), r.PgSumBlock, r.PgSumWin)
		fmt.Printf("差异:   字段差异 %d 条（数值不等 %d / 仅文本不同 %d）；点位缺失 聚合器独有 %d 个、PG 独有 %d 个\n",
			r.DiffCount, r.ValueDiffCount, r.FormatOnlyDiff, r.AggOnlyCount(), r.PgOnlyCount())
		if len(r.Unresolved) > 0 {
			fmt.Printf("无 PG 来源字段: %s\n", strings.Join(r.Unresolved, "; "))
		}
		for _, d := range r.Diffs {
			kind := "数值不等"
			if d.ValueEq {
				kind = "仅文本不同"
			}
			fmt.Printf("  [%s] %s @ %s: 聚合器=%s PG=%s\n", kind, d.Field, d.Key, d.Agg, d.Pg)
		}
		if len(r.AggOnly()) > 0 {
			fmt.Printf("  只在聚合器侧存在（PG 缺数据）样例: %s\n", strings.Join(r.AggOnly(), ", "))
		}
		if len(r.PgOnly()) > 0 {
			fmt.Printf("  只在 PG 侧存在（多余/重复）样例: %s\n", strings.Join(r.PgOnly(), ", "))
		}
		if opt.diagnose && !r.Pass(opt.strictFormat) {
			rewardparity.Diagnose(context.Background(), db, rewardparity.DiagnoseOptions{
				Miner: p.Miner.Address(),
				Start: p.Start.Int64(),
				End:   p.End.Int64(),
			}, os.Stdout)
		}
	}

	fmt.Println()
	if allPass {
		fmt.Printf("PARITY: PASS（%d 个端点全部逐字段一致%s）\n", len(results), strictHint(opt.strictFormat))
		return 0
	}
	fmt.Printf("PARITY: FAIL（存在差异，禁止开启 [feature] 对应开关）\n")
	return 1
}

func strictHint(strict bool) string {
	if strict {
		return "，含金额文本形态"
	}
	return "（金额文本形态未纳入判定：-strict-format=false）"
}

func emitJSON(results []rewardparity.Result, strict bool) int {
	type jsonResult struct {
		Endpoint       string   `json:"endpoint"`
		Pass           bool     `json:"pass"`
		AggRows        int      `json:"agg_rows"`
		PgRows         int      `json:"pg_rows"`
		DiffCount      int      `json:"diff_count"`
		ValueDiffCount int      `json:"value_diff_count"`
		FormatOnlyDiff int      `json:"format_only_diff_count"`
		AggOnlyCount   int      `json:"agg_only_count"`
		PgOnlyCount    int      `json:"pg_only_count"`
		Unresolved     []string `json:"unresolved_fields"`
		AggErr         string   `json:"agg_error,omitempty"`
		PgErr          string   `json:"pg_error,omitempty"`
		AggLatencyMs   float64  `json:"agg_latency_ms"`
		PgLatencyMs    float64  `json:"pg_latency_ms"`
		Diffs          []string `json:"diff_examples"`
		AggOnly        []string `json:"agg_only_examples"`
		PgOnly         []string `json:"pg_only_examples"`
	}
	out := make([]jsonResult, 0, len(results))
	allPass := true
	for _, r := range results {
		jr := jsonResult{
			Endpoint:       r.Endpoint,
			Pass:           r.Pass(strict),
			AggRows:        r.AggRows,
			PgRows:         r.PgRows,
			DiffCount:      r.DiffCount,
			ValueDiffCount: r.ValueDiffCount,
			FormatOnlyDiff: r.FormatOnlyDiff,
			AggOnlyCount:   r.AggOnlyCount(),
			PgOnlyCount:    r.PgOnlyCount(),
			Unresolved:     r.Unresolved,
			AggLatencyMs:   float64(r.AggLatency.Microseconds()) / 1000,
			PgLatencyMs:    float64(r.PgLatency.Microseconds()) / 1000,
			AggOnly:        r.AggOnly(),
			PgOnly:         r.PgOnly(),
		}
		if r.AggErr != nil {
			jr.AggErr = r.AggErr.Error()
		}
		if r.PgErr != nil {
			jr.PgErr = r.PgErr.Error()
		}
		for _, d := range r.Diffs {
			jr.Diffs = append(jr.Diffs, fmt.Sprintf("%s @ %s: agg=%s pg=%s value_eq=%v", d.Field, d.Key, d.Agg, d.Pg, d.ValueEq))
		}
		if !jr.Pass {
			allPass = false
		}
		out = append(out, jr)
	}
	body := map[string]interface{}{"tool": Name, "version": Version, "pass": allPass, "results": out}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(body); err != nil {
		fmt.Fprintf(os.Stderr, "输出 JSON 失败: %s\n", err)
		return 2
	}
	if allPass {
		return 0
	}
	return 1
}

func parseEndpoints(s string) ([]string, error) {
	valid := map[string]bool{}
	for _, e := range rewardparity.AllEndpoints() {
		valid[e] = true
	}
	var out []string
	for _, raw := range strings.Split(s, ",") {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		if !valid[e] {
			return nil, fmt.Errorf("未知端点 %q（可选：%s）", e, strings.Join(rewardparity.AllEndpoints(), ","))
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未指定任何端点")
	}
	return out, nil
}

func emptyAs(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
