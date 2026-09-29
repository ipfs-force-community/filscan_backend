package browser

import (
	"context"
	"time"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/types"
	"gorm.io/gorm"
)

// 本文件：把对外 API 的三个统计端点从「实时问聚合器（扇出 12 个冷库）」切到「读 PG 派生表」。
//
// 切换点是这里的一个**装饰器**（embeds londobell.Agg，只覆盖 3 个方法）：
//
//	modules/filscan/biz/browser/biz.go 的 NewBrowserBiz 首行聚合一次
//	⇒ IndexBiz / BlockChainBiz / AccountBiz / IMTokenBiz 拿到的 agg 都是被装饰后的，
//	  它们下面的 acl 层（AggAccountAcl / AggBlockChainAcl / AggIndexAcl）无需任何改动。
//
// 为什么在 API 侧包而不是在 injector.NewLondobellAgg（wire provider）里包：
//
//	同一个 provider 也被 filscan-syncer 使用，而同步器正是往这些表**写**的人；
//	在 provider 层切换会同时改到同步器的读行为（并牵动 4 份 wire_gen.go），风险不成比例。
//	NewBrowserBiz 只被 API（injector.NewBrowserAPI）调用，边界干净。
//
// 行为不变性（默认零变化）：
//   - 三个开关任一开启前，NewPgRewardAgg 原样返回入参 agg，不分配装饰器、不发 SQL；
//   - 开关开启后，只替换这三个端点的读路径，其余 100+ 个 agg 方法经 embedded interface 原样透传；
//   - PG 读失败（超时/连接/语法）**回落到聚合器**并打 Warn —— 宁慢不空，避免数据库抖动放大成接口故障。
//
// 与聚合器的逐字段对齐情况（口径依据见 dal 层文件头注释）：
//
//	miner_blockreward  : Epoch=epoch、TotalBlockReward=reward、BlockCount=block_count  —— 完全对齐
//	miners_blockreward : Epoch/Miner、TotalBlockReward=reward、BlockCount=block_count  —— 完全对齐
//	wincount           : Id=Miner、TotalWinCount=win_count、TotalGasReward=gas_reward  —— 完全对齐
//
// wincount 的 TotalGasReward（migration/36 起）：
//
//	来源是**同一个** /aggregators/wincount 响应里的 TotalGasReward（= Message.Detail.Params.GasReward，
//	即 RewardActor.AwardBlockReward 隐式消息 params 的 GasReward），同步器已从 36 号迁移起落库
//	（modules/syncer/chain/reward_task/reward_task.go 的 toMinerWinCount）。
//
//	它唯一的消费点是 acl_block_chain.GetBlockDetails（算 TxFeeReward / MinedReward，
//	assembler_block_chain_info.go:42-58），所以**绝不允许拿 0 顶未回填的行**：
//	读出来的行里只要还有 gas_reward IS NULL（migration/36 之前写入、回填脚本尚未覆盖的区间），
//	整个请求回落聚合器（宁慢不空）。判据见 incompleteGasRewardRow / SQLMinerWinCountsRange。
//	⇒ 未回填区间 = 今天的行为（走聚合器），已回填/新写入区间 = 走 PG。开开关不产生任何口径回归。

// PgRewardOptions 三个端点各自的开关 + PG 读超时。
type PgRewardOptions struct {
	MinerBlockReward  bool
	MinersBlockReward bool
	MinerWinCount     bool
	Timeout           time.Duration // <= 0 表示不设超时
}

// Enabled 是否至少有一个开关打开。全关时调用方应直接返回原 agg（零开销、零行为变化）。
func (o PgRewardOptions) Enabled() bool {
	return o.MinerBlockReward || o.MinersBlockReward || o.MinerWinCount
}

// PgRewardOptionsFromConfig 从配置读取开关。任何字段缺失 = 关闭（老配置行为不变）。
func PgRewardOptionsFromConfig(conf *config.Config) PgRewardOptions {
	opt := PgRewardOptions{
		MinerBlockReward:  conf.MinerBlockRewardReadFromPg(),
		MinersBlockReward: conf.MinersBlockRewardReadFromPg(),
		MinerWinCount:     conf.MinerWinCountReadFromPg(),
	}
	if ms := conf.PgReadTimeoutMs(); ms > 0 {
		opt.Timeout = time.Duration(ms) * time.Millisecond
	}
	return opt
}

// NewPgRewardAgg 按配置装饰 agg；开关全关（或 db 为空）时返回入参本身。
func NewPgRewardAgg(agg londobell.Agg, db *gorm.DB, conf *config.Config) londobell.Agg {
	opt := PgRewardOptionsFromConfig(conf)
	if !opt.Enabled() || db == nil {
		return agg
	}
	log.Infof("aggregator reward endpoints switched to PG: miner_blockreward=%v miners_blockreward=%v wincount=%v timeout=%s",
		opt.MinerBlockReward, opt.MinersBlockReward, opt.MinerWinCount, opt.Timeout)
	return NewPgRewardAggWithReader(agg, dal.NewMinerRewardRangeDal(db), opt)
}

// NewPgRewardAggWithReader 同上，但由调用方注入读实现（单测用）。
// 三个开关全关时同样原样返回入参：装饰器即使是「空转」也不该存在，
// 否则调用方会以为开了开关（读路径可观测性优先于少写一行判断）。
func NewPgRewardAggWithReader(agg londobell.Agg, reader repository.MinerRewardRange, opt PgRewardOptions) londobell.Agg {
	if !opt.Enabled() {
		return agg
	}
	return &pgRewardAgg{Agg: agg, reader: reader, opt: opt}
}

var _ londobell.Agg = (*pgRewardAgg)(nil)

// pgRewardAgg 只覆盖 3 个统计端点，其余方法经 embedded interface 原样透传给聚合器实现。
type pgRewardAgg struct {
	londobell.Agg
	reader repository.MinerRewardRange
	opt    PgRewardOptions
}

// withTimeout 为单次 PG 读派生一个带超时的 ctx；Timeout<=0 时原样返回（不设超时）。
func (a *pgRewardAgg) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if a.opt.Timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, a.opt.Timeout)
}

// MinerBlockReward 聚合器端点 /aggregators/miner_blockreward → chain.miner_rewards。
//
// 区间口径与聚合器一致：left-closed / right-open [filters.Start, filters.End)；
// filters 的 Index/Limit 与聚合器一样**不参与过滤**（线上管线的 match 阶段取的是
// StartEpoch/EndEpoch，没有 $skip/$limit 阶段）。
func (a *pgRewardAgg) MinerBlockReward(ctx context.Context, addr chain.SmartAddress, filters types.Filters) ([]*londobell.MinerBlockReward, error) {
	if !a.opt.MinerBlockReward {
		return a.Agg.MinerBlockReward(ctx, addr, filters)
	}
	if filters.Start == nil || filters.End == nil {
		// 没给区间就没法用 PG 的口径复现（聚合器此时会把 null 当 0 处理），保持老路径。
		return a.Agg.MinerBlockReward(ctx, addr, filters)
	}
	readCtx, cancel := a.withTimeout(ctx)
	rows, err := a.reader.MinerBlockRewardRange(readCtx, addr.Address(), *filters.Start, *filters.End)
	cancel()
	if err != nil {
		log.Warnf("read miner_blockreward from pg failed (miner=%s, [%d,%d)): %s; fallback to aggregator",
			addr.Address(), filters.Start.Int64(), filters.End.Int64(), err)
		return a.Agg.MinerBlockReward(ctx, addr, filters)
	}
	return minerEpochRewardsToMinerBlockReward(rows), nil
}

// MinersBlockReward 聚合器端点 /aggregators/miners_blockreward → chain.miner_rewards。
func (a *pgRewardAgg) MinersBlockReward(ctx context.Context, start chain.Epoch, end chain.Epoch) ([]*londobell.MinersBlockReward, error) {
	if !a.opt.MinersBlockReward {
		return a.Agg.MinersBlockReward(ctx, start, end)
	}
	readCtx, cancel := a.withTimeout(ctx)
	rows, err := a.reader.MinersBlockRewardRange(readCtx, start, end)
	cancel()
	if err != nil {
		log.Warnf("read miners_blockreward from pg failed ([%d,%d)): %s; fallback to aggregator", start.Int64(), end.Int64(), err)
		return a.Agg.MinersBlockReward(ctx, start, end)
	}
	return minerEpochRewardsToMinersBlockReward(rows), nil
}

// WinCount 聚合器端点 /aggregators/wincount → chain.miner_win_counts。
//
// 逐字段对齐：Id=Miner（不带前缀形态）、TotalWinCount=win_count、TotalGasReward=gas_reward。
//
// 未回填保护：只要本区间里还有 gas_reward IS NULL 的行（migration/36 之前写入、回填未覆盖），
// 就整请求回落聚合器 —— 因为 TotalGasReward 的消费点 acl_block_chain.GetBlockDetails
// 拿它算 TxFeeReward / MinedReward（assembler_block_chain_info.go:42-58），
// 用 0 顶过去等于把区块详情页的两个金额算错。
func (a *pgRewardAgg) WinCount(ctx context.Context, begin chain.Epoch, end chain.Epoch) ([]*londobell.MinerWinCount, error) {
	if !a.opt.MinerWinCount {
		return a.Agg.WinCount(ctx, begin, end)
	}
	readCtx, cancel := a.withTimeout(ctx)
	rows, err := a.reader.MinerWinCountsRange(readCtx, begin, end)
	cancel()
	if err != nil {
		log.Warnf("read wincount from pg failed ([%d,%d)): %s; fallback to aggregator", begin.Int64(), end.Int64(), err)
		return a.Agg.WinCount(ctx, begin, end)
	}
	if row := incompleteGasRewardRow(rows); row != nil {
		log.Warnf("read wincount from pg is incomplete ([%d,%d)): miner=%s has %d/%d rows without gas_reward (backfill pending); fallback to aggregator",
			begin.Int64(), end.Int64(), row.Miner, row.GasRewardRows, row.TotalRows)
		return a.Agg.WinCount(ctx, begin, end)
	}
	out := make([]*londobell.MinerWinCount, 0, len(rows))
	for _, row := range rows {
		out = append(out, &londobell.MinerWinCount{
			// 聚合器 wincount 的 _id 是 Message.Detail.Params.Miner（库内为**不带前缀**的 0… 形式），
			// 这里同样给 CrudeAddress()，避免两条路径在 Id 上出现形态差异。
			Id:             chain.SmartAddress(row.Miner).CrudeAddress(),
			TotalWinCount:  row.WinCount,
			TotalGasReward: gasRewardOf(row),
		})
	}
	if len(out) == 0 {
		// 与聚合器路径的空结果形态保持一致：聚合器返回空 data 数组时 JSON 反序列化得到 nil slice，
		// 调用方普遍用 `if x != nil` 判空（acl_account.go:348、acl_block_chain.go:400 等），
		// 这里回 nil 才不会把「无数据」变成「空但非 nil」这种可观测差异。
		return nil, nil
	}
	return out, nil
}

// incompleteGasRewardRow 找出第一条「gas_reward 不完整」的汇总行：
//
//	GasReward == nil        —— 该矿工在区间内所有行的 gas_reward 都是 NULL（sum 得 NULL）
//	GasRewardRows < TotalRows —— 该矿工在区间内部分行还没回填
//
// 返回 nil 表示整个区间的 gas_reward 都齐了，PG 结果可以直接用。
//
// 为什么是「全区间任一矿工不齐就整请求回落」而不是「只跳过这一行」：
// 调用方是 GetBlockDetails（一次请求一个高度、一个矿工），跳过会静默给出 0；
// 而区间内不同矿工混着走两条路径，会让同一份数据的 TxFeeReward 口径在高度之间不一致。
// 回落的代价只是慢（与开关打开前完全相同），不会错。
func incompleteGasRewardRow(rows []*bo.AccWinCount) *bo.AccWinCount {
	for _, row := range rows {
		if row == nil {
			continue
		}
		if row.GasReward == nil || row.GasRewardRows < row.TotalRows {
			return row
		}
	}
	return nil
}

// gasRewardOf 取汇总行的 gas_reward；理论上调用前已由 incompleteGasRewardRow 保证非 NULL，
// 这里再兜一层是为了「万一将来有人绕过检查直接调」时退化成 0 而不是 panic。
func gasRewardOf(row *bo.AccWinCount) decimal.Decimal {
	if row == nil || row.GasReward == nil {
		return decimal.Zero
	}
	return *row.GasReward
}

// minerEpochRewardsToMinerBlockReward 逐 epoch 行 → 聚合器 miner_blockreward 返回值。
// 空结果返回 nil（理由见 WinCount 的注释）。
func minerEpochRewardsToMinerBlockReward(rows []*bo.MinerEpochReward) []*londobell.MinerBlockReward {
	if len(rows) == 0 {
		return nil
	}
	out := make([]*londobell.MinerBlockReward, 0, len(rows))
	for _, row := range rows {
		out = append(out, &londobell.MinerBlockReward{
			Id:               row.Epoch,
			TotalBlockReward: row.Reward,
			BlockCount:       row.BlockCount,
		})
	}
	return out
}

// minerEpochRewardsToMinersBlockReward 逐 epoch 逐矿工行 → 聚合器 miners_blockreward 返回值。
func minerEpochRewardsToMinersBlockReward(rows []*bo.MinerEpochReward) []*londobell.MinersBlockReward {
	if len(rows) == 0 {
		return nil
	}
	out := make([]*londobell.MinersBlockReward, 0, len(rows))
	for _, row := range rows {
		out = append(out, &londobell.MinersBlockReward{
			Id: londobell.EpochMiner{
				Epoch: row.Epoch,
				Miner: chain.SmartAddress(row.Miner).CrudeAddress(),
			},
			TotalBlockReward: row.Reward,
			BlockCount:       row.BlockCount,
		})
	}
	return out
}
