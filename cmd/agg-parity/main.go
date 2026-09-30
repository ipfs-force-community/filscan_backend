// filscan-agg-parity：聚合器与 PG 两条读路径的一致性校验（上线前通关条件）。
//
// 覆盖端点：
//
//	三个统计端点 miner_blockreward / miners_blockreward / wincount      —— 按高度区间取数、逐字段比对
//	大额转账列表 large_amount（transfer_message_for_largeAmount）        —— 按页（index/limit）取数、逐行比对
//
// 用法（跑在 backend 主机上，用与 filscan-api 同一份 config.toml）：
//
//	# 1) 单矿工 + 区间：三个统计端点全比（wincount 会按 miner 过滤后比）
//	filscan-agg-parity -c /etc/filscan/config.toml -miner f01234 -start 4300000 -end 4300100
//
//	# 2) 只比某个端点；不指定 -miner 时 miners_blockreward / wincount 比**全矿工**（行多，区间选小一点）
//	filscan-agg-parity -c /etc/filscan/config.toml -start 4300000 -end 4300010 -endpoints miners_blockreward,wincount
//
//	# 3) 大额转账第 0 页（每页 50 条）：聚合器与 PG 逐行比 + TotalCount 对照
//	#    注意：线上该端点极慢（实测 HTTP 60 秒零字节、挂住）⇒ 聚合器侧默认超时 180s，可用 -agg-timeout 覆盖
//	filscan-agg-parity -c /etc/filscan/config.toml -endpoints large_amount -index 0 -limit 50
//
//	# 4) 聚合器不可用时只自检 PG 侧（不取聚合器、不消耗 180s 超时；仍会打印 PG 行数/TotalCount/定序检查）
//	filscan-agg-parity -c /etc/filscan/config.toml -endpoints large_amount -index 0 -limit 50 -pg-only
//
//	# 5) 输出 JSON（供上线 checklist / 工单留痕）
//	filscan-agg-parity -c /etc/filscan/config.toml -miner f01234 -start 4300000 -end 4300100 -endpoints miner_blockreward -json
//
// 退出码：0 = 全部端点 PASS；1 = 存在差异（数值不等 / 点位缺失 / 排序违规 / TotalCount 不等；
// 金额与地址的**文本形态**差异在 -strict-format=true 下也算差异）；
//
//	2 = 参数或初始化错误（配置/连库/聚合器地址）。
//
// 大额转账端点的两处**已知差异**（工具里默认不判失败，报告会分别计数）：
//
//	order_diff     同高度内行序：线上每个冷库只 $sort:{Epoch:-1}，同高度内行序未定义；
//	               PG 侧按 (epoch desc, cid asc) 定序。想严格按位置比对时用 -strict-order。
//	total_diff     TotalCount 来源不同（线上=各冷库区间行数求和，PG=全表 count(*)）。
//	               默认判失败；确认是聚合器计数陈旧时用 -lenient-total 降级为提示。
//
// 口径与字段映射见 modules/common/infra/dal/dal_biz_large_transfer.go、
// modules/common/infra/dal/dal_biz_miner_reward_range.go、
// modules/filscan/biz/browser/agg_pg_{reward,large_amount}.go 的文件头注释；
// 比对逻辑在 modules/filscan/service/rewardparity。
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

	// ===== 大额转账端点（large_amount）相关 =====
	index int64 // 页码（线上 skip = index*limit）
	limit int64 // 每页条数
	// aggTimeout 聚合器侧单次取数超时。该端点线上实测 60 秒零字节挂住 ⇒ 默认给足 180s，可用本参数覆盖。
	aggTimeout time.Duration
	// pgOnly 只跑 PG 侧（不调聚合器）：聚合器不可用/极慢时的自检通道。
	pgOnly bool
	// strictOrder 把「同高度内行序差异」也算失败（默认 false：线上该顺序本来未定义）。
	strictOrder bool
	// lenientTotal 把「两侧 TotalCount 不等」降级为提示（默认 false = 判失败）。
	lenientTotal bool
}

// DefaultAggTimeout 聚合器侧默认超时：该端点线上实测 60 秒零字节挂住，给足时间让人拿到结论。
const DefaultAggTimeout = 180 * time.Second

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
		"要比对的端点，逗号分隔："+strings.Join(rewardparity.ValidEndpoints(), ","))
	fs.IntVar(&opt.maxExamples, "max-examples", 10, "每类差异最多打印多少条样例（计数仍是全量）")
	fs.BoolVar(&opt.strictFormat, "strict-format", true,
		"金额/地址的**文本形态**是否也算失败：true（默认）= 只要两边的十进制/地址原文不同就判 FAIL")
	fs.BoolVar(&opt.jsonOut, "json", false, "以 JSON 输出（便于留痕/机检）")
	fs.BoolVar(&opt.diagnose, "diag", true, "出现差异时额外打印只读诊断（覆盖范围/重复行/地址形态）")
	fs.DurationVar(&opt.timeout, "timeout", 0, "PG 侧单次取数超时；<=0 取配置 [feature].pg_read_timeout_ms（默认 5s）")
	fs.BoolVar(&opt.showSQL, "print-sql", false, "打印 PG 读路径使用的 SQL（用于与聚合器管线逐条对照）")
	fs.BoolVar(&opt.showVersion, "version", false, "打印版本后退出")

	// 大额转账端点（large_amount）
	fs.Int64Var(&opt.index, "index", 0, "大额转账端点：页码（0 起；线上 skip = index*limit）")
	fs.Int64Var(&opt.limit, "limit", 20, "大额转账端点：每页条数（index 与 limit 都为 0 时线上取全量，慎用）")
	fs.DurationVar(&opt.aggTimeout, "agg-timeout", DefaultAggTimeout,
		"聚合器侧单次取数超时（大额转账端点线上极慢，默认 180s；<=0 取内置默认）")
	fs.BoolVar(&opt.pgOnly, "pg-only", false,
		"只跑 PG 侧自检（不调聚合器；聚合器不可用/极慢时用这个拿 PG 行数、TotalCount 与定序检查）")
	fs.BoolVar(&opt.strictOrder, "strict-order", false,
		"大额转账端点：同高度内行序差异是否也算失败（默认 false —— 线上该顺序本来未定义）")
	fs.BoolVar(&opt.lenientTotal, "lenient-total", false,
		"大额转账端点：两侧 TotalCount 不等降级为提示（默认 false = 判失败）")
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

	eps, err := parseEndpoints(opt.endpoints)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	// -start/-end 只对三个统计端点有意义；只比大额转账时不强求区间（它按 index/limit 取数）。
	if needsEpochRange(eps) {
		if opt.start < 0 || opt.end < 0 || opt.end <= opt.start {
			fmt.Fprintln(os.Stderr, "区间非法：需要 0 <= start < end（区间语义为左闭右开 [start, end)）")
			return 2
		}
	}
	if containsEndpoint(eps, rewardparity.EndpointLargeAmount) && opt.limit < 0 {
		fmt.Fprintln(os.Stderr, "-limit 不能为负（0 与 -index 0 同时出现时线上取全量，慎用）")
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
	aggTimeout := opt.aggTimeout
	if aggTimeout <= 0 {
		aggTimeout = DefaultAggTimeout
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
	largeReader := dal.NewLargeTransferDal(db)

	// 外层 ctx 的时限必须**不低于**聚合器侧超时，否则大额转账那一路会被 PG 的 5s 默认值提前砍掉。
	outer := timeout
	if !opt.pgOnly && aggTimeout > outer {
		outer = aggTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), outer)
	defer cancel()

	if !opt.jsonOut {
		fmt.Printf("%s %s\n", Name, Version)
		fmt.Printf("聚合器: %s\n", *conf.Londobell.AggAddress)
		fmt.Printf("区间:   [%d, %d) 左闭右开（聚合器管线条件是 Epoch>=$gte && Epoch<$lt）；矿工: %s；PG 读超时: %s\n",
			opt.start, opt.end, emptyAs(miner.Address(), "<全矿工>"), timeout)
		fmt.Printf("大额转账: index=%d limit=%d（折算 [%d, %d) 条）页码 0 起，线上 skip = index*limit；%s；聚合器超时: %s\n",
			opt.index, opt.limit, largeOffset(opt.index, opt.limit), opt.limit,
			pgOnlyNote(opt.pgOnly), aggTimeout)
		fmt.Printf("区间端点口径: PG 侧先 DISTINCT ON(epoch, miner) 去重再聚合；miner 列在库内是**带前缀**的 f0… 形态\n")
		fmt.Println("大额口径: 表 chain.large_transfers 只装 $match 命中行（ExitCode=0 + FIL>=10000）；" +
			"PG 侧 order by epoch desc, cid asc；TotalCount = 全表 count(*)（不去重、不按区间过滤）")
		fmt.Println("取数:   聚合器侧**始终直连**（绕过 API 读路径的 [feature] 开关）⇒ 结论与开关当前状态无关")
		if opt.showSQL {
			fmt.Println("PG 读路径 SQL:")
			for _, s := range []string{
				dal.SQLMinerBlockRewardRange, dal.SQLMinersBlockRewardRange, dal.SQLMinerWinCountsRange,
				dal.SQLLargeTransfersPage, dal.SQLCountLargeTransfers,
			} {
				fmt.Println(strings.TrimSpace(s))
				fmt.Println("---")
			}
		}
	}

	results := make([]rewardparity.Result, 0, len(eps))
	for _, ep := range eps {
		r := compareEndpoint(ctx, ep, params, agg, reader, largeReader, opt, timeout)
		results = append(results, r)
	}

	if opt.jsonOut {
		return emitJSON(results, opt)
	}
	return emitText(results, opt, db, params)
}

// compareEndpoint 取两侧数据并比对。pgTimeout 只用于大额转账端点（它的两次 PG 读各自套一个超时，
// 三个统计端点沿用外层 ctx，行为与改造前一致）。
func compareEndpoint(
	ctx context.Context,
	ep string,
	p rewardparity.Params,
	agg londobell.Agg,
	reader repository.MinerRewardRange,
	largeReader repository.LargeAmountTransfer,
	opt options,
	pgTimeout time.Duration,
) rewardparity.Result {
	maxEx := rewardparity.MaxExamples(opt.maxExamples)

	switch ep {
	case rewardparity.EndpointLargeAmount:
		return compareLargeAmountEndpoint(ctx, p, agg, largeReader, opt, pgTimeout)

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

// compareLargeAmountEndpoint 大额转账端点的两侧取数。
//
// 与三个统计端点的三点不同：
//
//  1. 请求只有 index/limit（没有高度区间）⇒ 两边都取「同一页」；PG 侧 offset = index*limit；
//  2. 聚合器侧默认超时给足（线上该端点实测 60 秒零字节挂住），且 -pg-only 时**完全不调**聚合器；
//  3. PG 侧两次查询（列表 + 计数）各自套一个超时。
func compareLargeAmountEndpoint(
	ctx context.Context,
	p rewardparity.Params,
	agg londobell.Agg,
	largeReader repository.LargeAmountTransfer,
	opt options,
	pgTimeout time.Duration,
) rewardparity.Result {
	maxEx := rewardparity.MaxExamples(opt.maxExamples)
	offset, pageLimit := dal.LargeTransferPageWindow(opt.index, opt.limit)
	laParams := rewardparity.LargeAmountParams{
		Index: opt.index, Limit: opt.limit, Offset: offset, PageLimit: pageLimit, PgOnly: opt.pgOnly,
	}

	var pgRows []*bo.LargeTransferRow
	var pgErr error
	pgStart := time.Now()
	func() {
		readCtx, cancel := withTimeout(ctx, pgTimeout)
		defer cancel()
		pgRows, pgErr = largeReader.LargeTransfersPage(readCtx, offset, pageLimit)
	}()
	var pgTotal int64
	if pgErr == nil {
		func() {
			readCtx, cancel := withTimeout(ctx, pgTimeout)
			defer cancel()
			pgTotal, pgErr = largeReader.CountLargeTransfers(readCtx)
		}()
	}
	pgLatency := time.Since(pgStart)

	var aggList *londobell.TransferLargeAmountList
	var aggErr error
	var aggLatency time.Duration
	if !opt.pgOnly {
		aggCtx, cancel := withTimeout(ctx, opt.aggTimeout)
		defer cancel()
		aggStart := time.Now()
		aggList, aggErr = agg.TransferLargeAmount(aggCtx, types.Filters{Index: opt.index, Limit: opt.limit})
		aggLatency = time.Since(aggStart)
	}

	r := rewardparity.CompareLargeAmount(laParams, aggList, pgRows, pgTotal, maxEx)
	r.Params = p
	r.AggErr, r.PgErr = aggErr, pgErr
	r.AggLatency, r.PgLatency = aggLatency, pgLatency
	return r
}

// withTimeout 派生一个带超时的 ctx；d<=0 时原样返回（不设超时）。
func withTimeout(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	if d <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, d)
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
		pass := r.PassDetailed(opt.strictFormat, !opt.lenientTotal, opt.strictOrder)
		verdict := "PASS"
		if !pass {
			verdict = "FAIL"
			allPass = false
		}
		if r.PgOnlyMode {
			verdict += "（-pg-only：未与聚合器比对）"
		}
		fmt.Printf("\n===== %s: %s =====\n", r.Endpoint, verdict)
		if r.AggErr != nil {
			fmt.Printf("聚合器侧错误: %s\n", r.AggErr)
		}
		if r.PgErr != nil {
			fmt.Printf("PG 侧错误:     %s\n", r.PgErr)
		}
		if r.Endpoint == rewardparity.EndpointLargeAmount {
			fmt.Printf("行数:   聚合器 %d 行 (%.0fms)  vs  PG %d 行 (%.0fms)\n",
				r.AggRows, float64(r.AggLatency.Microseconds())/1000, r.PgRows, float64(r.PgLatency.Microseconds())/1000)
			totalNote := ""
			switch {
			case r.PgOnlyMode:
				totalNote = "（-pg-only：没有聚合器侧对照值）"
			case r.TotalDiff:
				totalNote = "  ← 不等（线上=各冷库区间行数求和，PG=全表 count(*)）"
			}
			fmt.Printf("TotalCount: 聚合器 %d  vs  PG %d%s\n", r.AggTotal, r.PgTotal, totalNote)
			fmt.Printf("差异:   字段差异 %d 条（数值不等 %d / 仅文本不同 %d）；行缺失 聚合器独有 %d 行、PG 独有 %d 行\n",
				r.DiffCount, r.ValueDiffCount, r.FormatOnlyDiff, r.AggOnlyCount(), r.PgOnlyCount())
			fmt.Printf("已知差异: 同高度内行序 order_diff %d 条（线上未定义，默认不判失败；-strict-order 可升级）\n",
				r.OrderDiffCount)
			fmt.Printf("PG 自检: epoch 倒序违规 %d 条（PG 定序失效才会非 0，永远判失败）\n", r.SortViolations)
		} else {
			fmt.Printf("行数:   聚合器 %d 行 (%.0fms)  vs  PG %d 行 (%.0fms)\n",
				r.AggRows, float64(r.AggLatency.Microseconds())/1000, r.PgRows, float64(r.PgLatency.Microseconds())/1000)
			fmt.Printf("总量:   聚合器 reward=%s block=%d win=%d gasReward=%s\n",
				r.AggSumReward.String(), r.AggSumBlock, r.AggSumWin, r.AggSumGas.String())
			fmt.Printf("        PG     reward=%s block=%d win=%d gasReward=%s\n",
				r.PgSumReward.String(), r.PgSumBlock, r.PgSumWin, r.PgSumGas.String())
			fmt.Printf("差异:   字段差异 %d 条（数值不等 %d / 仅文本不同 %d）；点位缺失 聚合器独有 %d 个、PG 独有 %d 个\n",
				r.DiffCount, r.ValueDiffCount, r.FormatOnlyDiff, r.AggOnlyCount(), r.PgOnlyCount())
		}
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
		for _, s := range r.OrderDiffs {
			fmt.Printf("  [同高度内行序] %s\n", s)
		}
		for _, s := range r.SortSamples {
			fmt.Printf("  [PG 定序违规] %s\n", s)
		}
		if r.Endpoint == rewardparity.EndpointLargeAmount {
			if r.AggErr != nil && !opt.pgOnly {
				fmt.Println("  提示: 聚合器侧报错/超时时可用 -pg-only 只自检 PG 侧（该端点线上实测 60 秒零字节挂住，可配合 -agg-timeout）")
			}
			if !pass && (r.AggOnlyCount() > 0 || r.PgOnlyCount() > 0) {
				fmt.Println("  提示: 若差异全部集中在同一高度组内 ⇒ 属已知的「同高度内行序」差异（窗口切在高度组中间），" +
					"换一个 -index/-limit 让窗口不切断高度组再复核")
			}
			if r.TotalDiff {
				fmt.Println("  提示: TotalCount 不等时先用 -pg-only 核对 PG 侧全表行数（期望值=装载时点实测 245116，" +
					"见 migration/35.large_transfers.sql 的体量注释；表继续装载就会大于它），" +
					"并注意线上计数来自另一条管线、可能陈旧；确认无误后用 -lenient-total 降级为提示")
			}
		}
		if opt.diagnose && !pass && rewardparity.NeedsEpochRange(r.Endpoint) {
			rewardparity.Diagnose(context.Background(), db, rewardparity.DiagnoseOptions{
				Miner: p.Miner.Address(),
				Start: p.Start.Int64(),
				End:   p.End.Int64(),
			}, os.Stdout)
		}
	}

	fmt.Println()
	if allPass {
		fmt.Printf("PARITY: PASS（%d 个端点全部一致%s%s）\n", len(results),
			strictHint(opt.strictFormat), largeAmountHint(opt))
		return 0
	}
	fmt.Printf("PARITY: FAIL（存在差异，禁止开启 [feature] 对应开关）\n")
	return 1
}

// largeAmountHint 把大额转账端点的两个降级开关状态写进结论行（留痕时一眼看出判据）。
func largeAmountHint(opt options) string {
	if opt.strictOrder && opt.lenientTotal {
		return "；大额转账：严格行序 + 忽略 TotalCount 差额"
	}
	if opt.strictOrder {
		return "；大额转账：严格行序（含同高度内行序）"
	}
	if opt.lenientTotal {
		return "；大额转账：TotalCount 差额仅提示"
	}
	return ""
}

func strictHint(strict bool) string {
	if strict {
		return "，含金额文本形态"
	}
	return "（金额文本形态未纳入判定：-strict-format=false）"
}

func emitJSON(results []rewardparity.Result, opt options) int {
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

		// 大额转账端点（large_amount）专有字段
		PgOnlyMode     bool     `json:"pg_only_mode,omitempty"`
		AggTotal       int64    `json:"agg_total,omitempty"`
		PgTotal        int64    `json:"pg_total,omitempty"`
		TotalDiff      bool     `json:"total_diff,omitempty"`
		OrderDiffCount int      `json:"order_diff_count,omitempty"`
		OrderDiffs     []string `json:"order_diff_examples,omitempty"`
		SortViolations int      `json:"sort_violations,omitempty"`
		SortSamples    []string `json:"sort_violation_examples,omitempty"`
	}
	out := make([]jsonResult, 0, len(results))
	allPass := true
	for _, r := range results {
		pass := r.PassDetailed(opt.strictFormat, !opt.lenientTotal, opt.strictOrder)
		jr := jsonResult{
			Endpoint:       r.Endpoint,
			Pass:           pass,
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

			PgOnlyMode:     r.PgOnlyMode,
			AggTotal:       r.AggTotal,
			PgTotal:        r.PgTotal,
			TotalDiff:      r.TotalDiff,
			OrderDiffCount: r.OrderDiffCount,
			OrderDiffs:     r.OrderDiffs,
			SortViolations: r.SortViolations,
			SortSamples:    r.SortSamples,
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
	body := map[string]interface{}{
		"tool": Name, "version": Version, "pass": allPass, "results": out,
		"criteria": map[string]interface{}{
			"strict_format":          opt.strictFormat,
			"strict_order":           opt.strictOrder,
			"strict_total":           !opt.lenientTotal,
			"pg_only":                opt.pgOnly,
			"large_amount_index":     opt.index,
			"large_amount_limit":     opt.limit,
			"large_amount_pg_offset": largeOffset(opt.index, opt.limit),
			"agg_timeout_seconds":    opt.aggTimeout.Seconds(),
		},
	}
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

// largeOffset 大额转账端点的 PG offset（= index*limit；index=limit=0 时线上取全量，offset 为 0）。
func largeOffset(index, limit int64) int64 {
	offset, _ := dal.LargeTransferPageWindow(index, limit)
	return offset
}

// pgOnlyNote 头部口径行的一小段（把 -pg-only 的状态写清楚，避免把「没比」看成「比过了」）。
func pgOnlyNote(pgOnly bool) string {
	if pgOnly {
		return "**只跑 PG 侧自检，未调聚合器**"
	}
	return "两侧都比"
}

func parseEndpoints(s string) ([]string, error) {
	valid := map[string]bool{}
	for _, e := range rewardparity.ValidEndpoints() {
		valid[e] = true
	}
	var out []string
	for _, raw := range strings.Split(s, ",") {
		e := strings.TrimSpace(raw)
		if e == "" {
			continue
		}
		if !valid[e] {
			return nil, fmt.Errorf("未知端点 %q（可选：%s）", e, strings.Join(rewardparity.ValidEndpoints(), ","))
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未指定任何端点")
	}
	return out, nil
}

// needsEpochRange 给定的端点集合里是否有需要 -start/-end 的（三个统计端点需要，大额转账不需要）。
func needsEpochRange(endpoints []string) bool {
	for _, ep := range endpoints {
		if rewardparity.NeedsEpochRange(ep) {
			return true
		}
	}
	return false
}

func containsEndpoint(endpoints []string, target string) bool {
	for _, ep := range endpoints {
		if ep == target {
			return true
		}
	}
	return false
}

func emptyAs(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}
