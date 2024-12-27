package calc_miner_owner_task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/filecoin-project/go-state-types/builtin"
	logging "github.com/gozelle/logger"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/dal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/calculator/calc-miner-owner-task/luck"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader"
	"gorm.io/gorm"
)

var logger = logging.NewLogger("calc-miner-owner-task")

func NewCalcMinerOwnerTask(conf *config.Config, db *gorm.DB) *CalcMinerOwnerTask {
	return &CalcMinerOwnerTask{
		sg:   dal.NewSyncerDal(db),
		repo: dal.NewMinerTaskDal(db),
		conf: conf,
		luck: luck.NewCalculator(dal.NewLuckDal(db)),
	}
}

var _ syncer.Calculator = (*CalcMinerOwnerTask)(nil)

type CalcMinerOwnerTask struct {
	repo    repository.MinerTask
	sg      repository.SyncerGetter
	conf    *config.Config
	luck    *luck.Calculator
	GapScan bool
}

func (m CalcMinerOwnerTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (m CalcMinerOwnerTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = m.repo.DeleteSyncMinerEpochs(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = m.repo.DeleteMinerStats(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = m.repo.DeleteOwnerStats(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (m CalcMinerOwnerTask) Name() string {
	return "calc-miner-owner-task"
}

func (m CalcMinerOwnerTask) Calc(ctx *syncer.Context) (err error) {

	if m.GapScan {
		var infos []*po.MinerInfo
		infos, err = m.repo.GetMinerInfosByEpoch(ctx.Context(), ctx.Epoch())
		if err != nil {
			return
		}
		if len(infos) > 0 {
			ctx.Infof("该高度已存在记录，忽略同步")
			return
		}
	}

	if !m.conf.TestNet {
		if ctx.Epoch()%120 != 0 {
			return
		}
	}

	for {
		var s *po.SyncSyncer
		s, err = m.sg.GetSyncer(ctx.Context(), syncer.ChainSyncer)
		if err != nil {
			return
		}
		if s.Epoch >= ctx.Epoch().Int64() {
			break
		}
		wait := 15 * time.Second
		ctx.Debugf("Chain 同步器高度未到，等待: %s", wait)
		time.Sleep(wait)
	}

	minerInfos, err := m.repo.GetMinerInfosByEpoch(ctx.Context(), ctx.Epoch())
	if err != nil {
		return
	}
	ctx.Debugf("miner infos length: %d", len(minerInfos))
	if len(minerInfos) == 0 {
		return
	}

	var netQualityAdjPower decimal.Decimal
	if !m.GapScan {
		netQualityAdjPower, err = m.GetNetQualityAdjPower(ctx)
		if err != nil {
			err = fmt.Errorf("获取全网算力错误: %s", err)
			return
		}
	}

	minerStats24h, minerStats7d, minerStats30d, minerStats1y, err := m.handlerMinerStats(ctx.Context(), ctx.Epoch(), netQualityAdjPower, minerInfos)
	if err != nil {
		return
	}

	ownerStats24h, ownerStats7d, ownerStats30d, ownerStats1y, err := m.convertOwnerStats(ctx.Epoch(), minerInfos, minerStats24h, minerStats7d, minerStats30d, minerStats1y)
	if err != nil {
		return
	}

	err = m.save(
		ctx.Context(),
		ctx.Epoch(),
		minerStats24h,
		minerStats7d,
		minerStats30d,
		minerStats1y,
		ownerStats24h,
		ownerStats7d,
		ownerStats30d,
		ownerStats1y,
	)
	if err != nil {
		return
	}

	// 2880为一天的出块量,2160根据第0块时间算出
	if ctx.Epoch()%2880 == 2160 {
		for k := range minerStats24h {
			minerStats24h[k].Interval = "2880"
		}
		for k := range ownerStats24h {
			ownerStats24h[k].Interval = "2880"
		}
		err = m.saveByDay(
			ctx.Context(),
			minerStats24h,
			ownerStats24h,
		)
		if err != nil {
			return
		}
	}

	return
}

func (m CalcMinerOwnerTask) GetNetQualityAdjPower(ctx *syncer.Context) (power decimal.Decimal, err error) {
	epoch := ctx.Epoch()
	powerActor, err := ctx.Adapter().Actor(ctx.Context(), chain.SmartAddress(builtin.StoragePowerActorAddr.String()), &epoch)
	if err != nil {
		return
	}
	d, err := json.Marshal(powerActor.State)
	if err != nil {
		return
	}

	state, err := upgrader.UnmarshalerPowerState(d)
	if err != nil {
		return
	}
	power = decimal.NewFromBigInt(state.TotalQualityAdjPower.Int, 0)
	return
}

func (m CalcMinerOwnerTask) handlerMinerStats(ctx context.Context, epoch chain.Epoch, netQualityAdjPower decimal.Decimal, minerInfos []*po.MinerInfo) (minerStats24h, minerStats7d, minerStats30d, minerStats1y []*po.MinerStat, err error) {

	// 处理 Miner 1y 变化
	if epoch%2880 == 2160 {
		prevEpoch := epoch - 2880*365
		var stats []*po.MinerStat
		stats, err = m.handleMinerChangePower(ctx, "1y", prevEpoch, epoch, minerInfos)
		if err != nil {
			return
		}
		err = m.handleMinerAccReward(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccWinCount(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccGasFee(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		now := time.Now()
		err = m.calcLuckRates(ctx, epoch, "1y", stats)
		if err != nil {
			return
		}
		minerStats1y = stats
		logger.Infof("1y luck elapsed time:%s", time.Since(now).String())
	}

	// 处理 Miner 24h 变化
	{
		prevEpoch := epoch - 2880
		var stats []*po.MinerStat
		stats, err = m.handleMinerChangePower(ctx, "24h", prevEpoch, epoch, minerInfos)
		if err != nil {
			return
		}
		err = m.handleMinerAccReward(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccWinCount(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccGasFee(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.calcLuckRates(ctx, epoch, "24h", stats)
		if err != nil {
			return
		}
		minerStats24h = stats
	}

	// 处理 Miner 7d 变化
	{
		prevEpoch := epoch - 2880*7
		var stats []*po.MinerStat
		stats, err = m.handleMinerChangePower(ctx, "7d", prevEpoch, epoch, minerInfos)
		if err != nil {
			return
		}
		err = m.handleMinerAccReward(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccWinCount(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccGasFee(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.calcLuckRates(ctx, epoch, "7d", stats)
		if err != nil {
			return
		}
		minerStats7d = stats
	}

	// 处理 Miner 30d 变化
	{
		prevEpoch := epoch - 2880*30
		var stats []*po.MinerStat
		stats, err = m.handleMinerChangePower(ctx, "30d", prevEpoch, epoch, minerInfos)
		if err != nil {
			return
		}
		err = m.handleMinerAccReward(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccWinCount(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.handleMinerAccGasFee(ctx, prevEpoch, epoch, stats)
		if err != nil {
			return
		}
		err = m.calcLuckRates(ctx, epoch, "30d", stats)
		if err != nil {
			return
		}
		minerStats30d = stats
	}

	return
}

func (m CalcMinerOwnerTask) handleMinerChangePower(ctx context.Context, interval string, prevEpoch, epoch chain.Epoch, items []*po.MinerInfo) (stats []*po.MinerStat, err error) {

	totalQualityAdjPower := decimal.Decimal{}
	for _, entity := range items {
		totalQualityAdjPower = totalQualityAdjPower.Add(entity.QualityAdjPower)
	}

	prevInfos, err := m.repo.GetMinerInfosByEpoch(ctx, prevEpoch)
	if err != nil {
		return
	}
	prevInfosMap := map[string]*po.MinerInfo{}
	for _, v := range prevInfos {
		prevInfosMap[v.Miner] = v
	}

	for _, item := range items {
		item.QualityAdjPowerPercent = item.QualityAdjPower.Div(totalQualityAdjPower)
		stat := &po.MinerStat{
			Epoch:        epoch.Int64(),
			Interval:     interval,
			Miner:        item.Miner,
			PrevEpochRef: prevEpoch.Int64(),
		}
		stat.SetSectorSize(item.SectorSize)
		stat.SetQualityAdjPower(item.QualityAdjPower)
		err = m.handleMinerChangePowerValue(stat, item, prevInfosMap)
		if err != nil {
			return
		}
		stats = append(stats, stat)
	}
	return
}

func (m CalcMinerOwnerTask) handleMinerChangePowerValue(stat *po.MinerStat, item *po.MinerInfo, prevInfosMap map[string]*po.MinerInfo) (err error) {

	prevInfo := prevInfosMap[item.Miner]

	stat.RawBytePowerChange = item.RawBytePower
	stat.QualityAdjPowerChange = item.QualityAdjPower
	stat.SectorCountChange = item.SectorCount
	stat.InitialPledgeChange = item.InitialPledge

	if prevInfo != nil {
		stat.RawBytePowerChange = stat.RawBytePowerChange.Sub(prevInfo.RawBytePower)
		stat.QualityAdjPowerChange = stat.QualityAdjPowerChange.Sub(prevInfo.QualityAdjPower)
		stat.SectorCountChange = stat.SectorCountChange - prevInfo.SectorCount
		stat.InitialPledgeChange = stat.InitialPledgeChange.Sub(prevInfo.InitialPledge)
	}

	return
}

func (m CalcMinerOwnerTask) handleMinerAccReward(ctx context.Context, prevEpoch, epoch chain.Epoch, stats []*po.MinerStat) (err error) {

	totalReward := decimal.Decimal{}
	totalBlockCount := int64(0)
	rewards, err := m.repo.GetMinersAccRewards(ctx, chain.NewLORCRange(prevEpoch, epoch))
	if err != nil {
		return
	}

	rewardsMap := map[string]*bo.AccReward{}

	for _, v := range rewards {
		totalReward = totalReward.Add(v.Reward)
		totalBlockCount += v.BlockCount
		rewardsMap[v.Miner] = v
	}

	for _, stat := range stats {
		stat.PrevEpochRef = prevEpoch.Int64()
		if v, ok := rewardsMap[stat.Miner]; ok {
			stat.AccReward, stat.AccBlockCount = v.Reward, v.BlockCount
		}
		if totalReward.GreaterThan(decimal.Zero) {
			stat.AccRewardPercent = stat.AccReward.Div(totalReward)
		}
		if totalBlockCount > 0 {
			stat.AccBlockCountPercent = decimal.NewFromInt(stat.AccBlockCount).Div(decimal.NewFromInt(totalBlockCount))
		}
		if stat.QualityAdjPower().GreaterThan(decimal.Zero) {
			stat.RewardPowerRatio = stat.AccReward.Div(stat.QualityAdjPower().Div(chain.PerT))
		}
	}

	return
}

func (m CalcMinerOwnerTask) handleMinerAccWinCount(ctx context.Context, prevEpoch, epoch chain.Epoch, stats []*po.MinerStat) (err error) {

	winCounts, err := m.repo.GetMinersAccWinCount(ctx, chain.NewLORCRange(prevEpoch, epoch))
	if err != nil {
		return
	}

	winCountsMap := map[string]*bo.AccWinCount{}
	totalWinCount := decimal.Decimal{}
	for _, v := range winCounts {
		winCountsMap[v.Miner] = v
		totalWinCount = totalWinCount.Add(decimal.NewFromInt(v.WinCount))
	}

	for _, stat := range stats {
		if v, ok := winCountsMap[stat.Miner]; ok {
			stat.AccWinCount = v.WinCount
			if totalWinCount.GreaterThan(decimal.Zero) {
				stat.WiningRate = decimal.NewFromInt(v.WinCount).Div(totalWinCount)
			}
		}
	}

	return
}

func (m CalcMinerOwnerTask) handleMinerAccGasFee(ctx context.Context, prevEpoch, epoch chain.Epoch, stats []*po.MinerStat) (err error) {

	gasFees, err := m.repo.GetMinersAccGasFees(ctx, chain.NewLORCRange(prevEpoch, epoch))
	if err != nil {
		return
	}

	gasFeesMap := map[string]*bo.AccGasFee{}

	for _, v := range gasFees {
		gasFeesMap[v.Miner] = v
	}

	for _, stat := range stats {
		if v, ok := gasFeesMap[stat.Miner]; ok {
			stat.AccSealGas = v.SealGas
			stat.AccWdPostGas = v.WdPostGas
		}
	}

	return
}

// 幸运值计算公式：epoch区间赢票数/（epoch数*5*算力/全网算力）
func (m CalcMinerOwnerTask) calcLuckRates(ctx context.Context, epoch chain.Epoch, interval string, stats []*po.MinerStat) (err error) {

	lucks, err := m.luck.CalcMinersLuckRate(ctx, epoch, interval)
	if err != nil {
		return
	}

	for _, v := range stats {
		v.LuckRate = lucks[v.Miner]
	}

	return
}

func (m CalcMinerOwnerTask) save(ctx context.Context, epoch chain.Epoch, minerStats24, minerStats7d, minerStats30d, minerStats1y []*po.MinerStat,
	ownerStats24h, ownerStats7d, ownerStats30d, ownerStats1y []*po.OwnerStat) (err error) {

	err = m.repo.SaveSyncMinerEpochPo(ctx, &po.SyncMinerEpochPo{
		Epoch:           epoch.Int64(),
		EffectiveMiners: int64(len(minerStats24)),
		Owners:          int64(len(ownerStats24h)),
	})

	if err != nil {
		return err
	}

	// 保存 Miner 统计
	{
		err = m.repo.SaveMinerStats(ctx, minerStats24)
		if err != nil {
			return
		}

		err = m.repo.SaveMinerStats(ctx, minerStats7d)
		if err != nil {
			return
		}

		err = m.repo.SaveMinerStats(ctx, minerStats30d)
		if err != nil {
			return
		}
	}

	// 保存 Owner 统计
	{
		err = m.repo.SaveOwnerStats(ctx, ownerStats24h)
		if err != nil {
			return
		}
		err = m.repo.SaveOwnerStats(ctx, ownerStats7d)
		if err != nil {
			return
		}
		err = m.repo.SaveOwnerStats(ctx, ownerStats30d)
		if err != nil {
			return
		}
	}

	if epoch%2880 == 2160 {
		err = m.repo.SaveMinerStats(ctx, minerStats1y)
		if err != nil {
			return
		}
		err = m.repo.SaveOwnerStats(ctx, ownerStats1y)
		if err != nil {
			return
		}
	}

	return
}

// 按天存储统计信息
func (m CalcMinerOwnerTask) saveByDay(ctx context.Context, minerStats []*po.MinerStat,
	ownerStats []*po.OwnerStat) (err error) {

	// 保存 Miner 统计
	{
		err = m.repo.SaveMinerStats(ctx, minerStats)
		if err != nil {
			return
		}
	}

	// 保存 Owner 统计
	{
		err = m.repo.SaveOwnerStats(ctx, ownerStats)
		if err != nil {
			return
		}
	}

	return
}

func (m CalcMinerOwnerTask) convertOwnerStats(epoch chain.Epoch, minerInfo []*po.MinerInfo, minerStats24h, minerStats7d, minerStats30d, minerStats1y []*po.MinerStat) (
	ownerStats24h, ownerStats7d, ownerStats30d, ownerStats1y []*po.OwnerStat, err error) {

	owners := map[string][]*po.MinerInfo{}
	totalPower := decimal.Decimal{}
	for _, v := range minerInfo {
		totalPower = totalPower.Add(v.QualityAdjPower)
		if _, ok := owners[v.Owner]; !ok {
			owners[v.Owner] = []*po.MinerInfo{}
		}
		owners[v.Owner] = append(owners[v.Owner], v)
	}

	ownerStats24h = m.mergeMinerStatToOwnerStat(epoch, owners, minerStats24h)
	ownerStats7d = m.mergeMinerStatToOwnerStat(epoch, owners, minerStats7d)
	ownerStats30d = m.mergeMinerStatToOwnerStat(epoch, owners, minerStats30d)
	if epoch%2880 == 2160 {
		ownerStats1y = m.mergeMinerStatToOwnerStat(epoch, owners, minerStats1y)
	}

	return
}

func (m CalcMinerOwnerTask) mergeMinerStatToOwnerStat(epoch chain.Epoch, rel map[string][]*po.MinerInfo, minerStats []*po.MinerStat) (ownerStats []*po.OwnerStat) {

	ownerStatsMap := map[string]*po.OwnerStat{}
	minerStatsMap := make(map[string]*po.MinerStat, len(minerStats))
	for _, v := range minerStats {
		minerStatsMap[v.Miner] = v
	}
	totalReward := decimal.Decimal{}
	totalBlockCount := int64(0)

	for k, v := range rel {
		if _, ok := ownerStatsMap[k]; !ok {
			ownerStatsMap[k] = &po.OwnerStat{
				Epoch: epoch.Int64(),
				Owner: k,
			}
		}
		ownerQualityAdjPower := decimal.Decimal{}
		for _, vv := range v {
			ownerQualityAdjPower = ownerQualityAdjPower.Add(vv.QualityAdjPower)
			minerStat := minerStatsMap[vv.Miner]
			totalReward = totalReward.Add(minerStat.AccReward)
			totalBlockCount += minerStat.AccBlockCount
			ownerStatsMap[k].Interval = minerStat.Interval
			ownerStatsMap[k].PrevEpochRef = minerStat.PrevEpochRef
			ownerStatsMap[k].RawBytePowerChange = ownerStatsMap[k].RawBytePowerChange.Add(minerStat.RawBytePowerChange)
			ownerStatsMap[k].QualityAdjPowerChange = ownerStatsMap[k].QualityAdjPowerChange.Add(minerStat.QualityAdjPowerChange)
			ownerStatsMap[k].SectorCountChange = ownerStatsMap[k].SectorCountChange + minerStat.SectorCountChange
			ownerStatsMap[k].SectorPowerChange = ownerStatsMap[k].SectorPowerChange.Add(decimal.NewFromInt(vv.SectorSize).Mul(decimal.NewFromInt(minerStat.SectorCountChange)))
			ownerStatsMap[k].InitialPledgeChange = ownerStatsMap[k].InitialPledgeChange.Add(minerStat.InitialPledgeChange)
			ownerStatsMap[k].AccReward = ownerStatsMap[k].AccReward.Add(minerStat.AccReward)
			ownerStatsMap[k].AccBlockCount = ownerStatsMap[k].AccBlockCount + minerStat.AccBlockCount
			ownerStatsMap[k].AccWinCount = ownerStatsMap[k].AccWinCount + minerStat.AccWinCount
			ownerStatsMap[k].AccSealGas = ownerStatsMap[k].AccSealGas.Add(minerStat.AccSealGas)
			ownerStatsMap[k].AccWdPostGas = ownerStatsMap[k].AccWdPostGas.Add(minerStat.AccWdPostGas)
		}
		if ownerQualityAdjPower.GreaterThan(decimal.Zero) {
			ownerStatsMap[k].RewardPowerRatio = ownerStatsMap[k].AccReward.Div(ownerQualityAdjPower.Div(chain.PerT))
		}
	}

	for _, v := range ownerStatsMap {
		if totalReward.GreaterThan(decimal.Zero) {
			v.AccRewardPercent = v.AccReward.Div(totalReward)
		}
		if totalBlockCount > 0 {
			v.AccBlockCountPercent = decimal.NewFromInt(v.AccBlockCount).Div(decimal.NewFromInt(totalBlockCount))
		}
		ownerStats = append(ownerStats, v)
	}

	return
}
