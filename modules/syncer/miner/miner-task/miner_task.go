package miner_task

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/filecoin-project/go-state-types/builtin"
	logging "github.com/gozelle/logger"
	"github.com/gozelle/mix"
	"github.com/multiformats/go-multiaddr"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/config"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

var log = logging.NewLogger("miner-task")

func NewMinerInfoTask(conf *config.Config, seg repository.SyncEpochGetter, repo repository.MinerTask) *MinerInfoTask {
	return &MinerInfoTask{seg: seg, repo: repo, conf: conf}
}

var _ syncer.Task = (*MinerInfoTask)(nil)

type MinerInfoTask struct {
	seg     repository.SyncEpochGetter
	repo    repository.MinerTask
	conf    *config.Config
	GapScan bool
}

func (m MinerInfoTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	//TODO implement me
	panic("implement me")
}

func (m MinerInfoTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = m.repo.DeleteMinerInfos(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = m.repo.DeleteOwnerInfos(ctx, gteEpoch)
	if err != nil {
		return
	}
	err = m.repo.DeleteAbsPower(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (m MinerInfoTask) Name() string {
	return "miner-task"
}

func (m MinerInfoTask) Exec(ctx *syncer.Context) (err error) {

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

	if ctx.Epoch()%120 == 0 {
		ctx.Infof("开始获取 miner infos 数据")
	}

	now := time.Now()
	infos, err := ctx.Agg().MinersInfo(ctx.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}
	ctx.Debugf("infos length: %d", len(infos))
	if c := time.Since(now); c > 3*time.Second {
		log.Warnf("调用 agg.MinersInfo 结束, infos: %d 耗时: %s", len(infos), c)
	}

	if len(infos) == 0 {
		if !m.conf.TestNet && ctx.Epoch()%120 == 0 {
			err = mix.Warnf("整点高度未获取到 miner 数据")
			return
		}
		return
	}

	var totalPower decimal.Decimal
	if !m.GapScan {
		totalPower, err = m.GetNetQualityAdjPower(ctx)
		if err != nil {
			return
		}
	}

	minerInfos, err := m.handlerMinerInfos(infos, totalPower)
	if err != nil {
		return
	}

	ownerInfos, err := m.convertOwnerInfos(ctx.Epoch(), minerInfos)
	if err != nil {
		return
	}

	// Miner 按有效算力排名
	sort.Sort(QualityAdjPowerMinersRank(minerInfos))
	for i, v := range minerInfos {
		v.QualityAdjPowerRank = int64(i + 1)
	}

	// Owner 按有效算力排名
	sort.Sort(QualityAdjPowerOwnersRank(ownerInfos))
	for i, v := range ownerInfos {
		v.QualityAdjPowerRank = int64(i + 1)
	}

	err = m.save(
		ctx.Context(),
		minerInfos,
		ownerInfos,
	)
	if err != nil {
		return
	}

	// 清理 1 天前历史数据
	clearEpoch := ctx.Epoch() - 2880
	err = m.repo.DeleteMinerStatsBeforeEpoch(ctx.Context(), clearEpoch)
	if err != nil {
		return
	}
	err = m.repo.DeleteOwnerStatsBeforeEpoch(ctx.Context(), clearEpoch)
	if err != nil {
		return
	}

	minerInfosLastHour, err := m.repo.GetMinerInfosByEpoch(ctx.Context(), ctx.Epoch()-120)
	if err != nil {
		return
	}

	powerIncrease, powerLoss := decimal.Zero, decimal.Zero
	mpNow := map[string]*po.MinerInfo{}
	for i := range minerInfos {
		powerIncrease = powerIncrease.Add(decimal.NewFromInt(minerInfos[i].SectorSize).Mul(decimal.NewFromInt(minerInfos[i].SectorCount)))
		powerLoss = powerLoss.Add(decimal.NewFromInt(minerInfos[i].SectorSize).Mul(decimal.NewFromInt(minerInfos[i].TerminateSectorCount)))
		mpNow[minerInfos[i].Miner] = minerInfos[i]
	}

	for i := range minerInfosLastHour {
		if v, ok := mpNow[minerInfosLastHour[i].Miner]; ok {
			if minerInfosLastHour[i].SectorCount <= v.SectorCount {
				powerIncrease = powerIncrease.Sub(decimal.NewFromInt(minerInfosLastHour[i].SectorSize).Mul(decimal.NewFromInt(minerInfosLastHour[i].SectorCount)))
			} else {
				powerIncrease = powerIncrease.Sub(decimal.NewFromInt(minerInfosLastHour[i].SectorSize).Mul(decimal.NewFromInt(v.SectorCount)))
			}
			if minerInfosLastHour[i].TerminateSectorCount <= v.TerminateSectorCount {
				powerLoss = powerLoss.Sub(decimal.NewFromInt(minerInfosLastHour[i].SectorSize).Mul(decimal.NewFromInt(minerInfosLastHour[i].TerminateSectorCount)))
			} else {
				powerLoss = powerLoss.Sub(decimal.NewFromInt(minerInfosLastHour[i].SectorSize).Mul(decimal.NewFromInt(v.TerminateSectorCount)))
			}
		} else {
			powerLoss = powerLoss.Add(decimal.NewFromInt(minerInfosLastHour[i].SectorSize).
				Mul(decimal.NewFromInt(minerInfosLastHour[i].SectorCount - minerInfosLastHour[i].TerminateSectorCount)))
		}
	}
	return m.repo.SaveAbsPower(ctx.Context(), powerIncrease, powerLoss, ctx.Epoch().Int64())
}

func (m MinerInfoTask) GetNetQualityAdjPower(ctx *syncer.Context) (power decimal.Decimal, err error) {
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

func (m MinerInfoTask) handlerMinerInfos(infos []*londobell.MinerInfo, totalPower decimal.Decimal) (minerInfos []*po.MinerInfo, err error) {

	for _, v := range infos {
		var minerInfo *po.MinerInfo
		minerInfo, err = m.ToMinerInfoPo(v, totalPower)
		if err != nil {
			err = fmt.Errorf("convert MinerInfoPo faild: %s", err)
			return
		}
		minerInfos = append(minerInfos, minerInfo)
	}

	return
}

func (m MinerInfoTask) save(ctx context.Context, minerInfos []*po.MinerInfo, ownerInfos []*po.OwnerInfo) (err error) {

	if len(minerInfos) > 0 {
		err = m.repo.SaveMinerInfos(ctx, minerInfos)
		if err != nil {
			return
		}
	}

	if len(ownerInfos) > 0 {
		err = m.repo.SaveOwnerInfos(ctx, ownerInfos)
		if err != nil {
			return
		}
	}

	return
}

func (m MinerInfoTask) convertOwnerInfos(epoch chain.Epoch, minerInfo []*po.MinerInfo) (
	ownerInfos []*po.OwnerInfo, err error) {

	owners := map[string][]*po.MinerInfo{}
	totalPower := decimal.Decimal{}
	for _, v := range minerInfo {
		totalPower = totalPower.Add(v.QualityAdjPower)
		if _, ok := owners[v.Owner]; !ok {
			owners[v.Owner] = []*po.MinerInfo{}
		}
		owners[v.Owner] = append(owners[v.Owner], v)
	}

	for k, v := range owners {
		var vv *po.OwnerInfo
		vv, err = m.ToOwnerInfoPo(epoch, chain.SmartAddress(k), v)
		if err != nil {
			return
		}
		if totalPower.GreaterThan(decimal.Zero) {
			vv.QualityAdjPowerPercent = vv.QualityAdjPower.Div(totalPower)
		}
		ownerInfos = append(ownerInfos, vv)
	}

	return
}

func (m MinerInfoTask) ToOwnerInfoPo(e chain.Epoch, o chain.SmartAddress, source []*po.MinerInfo) (target *po.OwnerInfo, err error) {
	target = &po.OwnerInfo{
		Epoch:                  e.Int64(),
		Owner:                  o.Address(),
		Miners:                 nil,
		RawBytePower:           decimal.Decimal{},
		QualityAdjPower:        decimal.Decimal{},
		Balance:                decimal.Decimal{},
		AvailableBalance:       decimal.Decimal{},
		VestingFunds:           decimal.Decimal{},
		FeeDebt:                decimal.Decimal{},
		SectorSize:             0,
		SectorCount:            0,
		FaultSectorCount:       0,
		ActiveSectorCount:      0,
		LiveSectorCount:        0,
		RecoverSectorCount:     0,
		TerminateSectorCount:   0,
		PreCommitSectorCount:   0,
		InitialPledge:          decimal.Decimal{},
		PreCommitDeposits:      decimal.Decimal{},
		QualityAdjPowerRank:    0,
		QualityAdjPowerPercent: decimal.Decimal{},
	}
	totalQualityAdjPower := decimal.Decimal{}
	for _, v := range source {
		totalQualityAdjPower = totalQualityAdjPower.Add(v.QualityAdjPower)
		target.Miners = append(target.Miners, v.Miner)
		target.Balance = target.Balance.Add(v.Balance)
		target.AvailableBalance = target.AvailableBalance.Add(v.AvailableBalance)
		target.VestingFunds = target.VestingFunds.Add(v.VestingFunds)
		target.FeeDebt = target.FeeDebt.Add(v.FeeDebt)
		target.SectorSize = target.SectorSize + v.SectorSize
		target.SectorCount = target.SectorCount + v.SectorCount
		target.FaultSectorCount = target.FaultSectorCount + v.FaultSectorCount
		target.ActiveSectorCount = target.ActiveSectorCount + v.ActiveSectorCount
		target.LiveSectorCount = target.LiveSectorCount + v.LiveSectorCount
		target.RecoverSectorCount = target.RecoverSectorCount + v.RecoverSectorCount
		target.TerminateSectorCount = target.TerminateSectorCount + v.TerminateSectorCount
		target.PreCommitSectorCount = target.PreCommitSectorCount + v.PreCommitSectorCount
		target.RawBytePower = target.RawBytePower.Add(v.RawBytePower)
		target.QualityAdjPower = target.QualityAdjPower.Add(v.QualityAdjPower)
		target.FaultSectorCount = target.FaultSectorCount + v.FaultSectorCount
		target.ActiveSectorCount = target.ActiveSectorCount + v.ActiveSectorCount
		target.LiveSectorCount = target.LiveSectorCount + v.LiveSectorCount
		target.RecoverSectorCount = target.RecoverSectorCount + v.RecoverSectorCount
		target.TerminateSectorCount = target.TerminateSectorCount + v.TerminateSectorCount
		target.PreCommitSectorCount = target.PreCommitSectorCount + v.PreCommitSectorCount
		target.InitialPledge = target.InitialPledge.Add(v.InitialPledge)
		target.PreCommitDeposits = target.PreCommitDeposits.Add(v.PreCommitDeposits)
	}

	return
}

func (m MinerInfoTask) ToMinerInfoPo(source *londobell.MinerInfo, totalPower decimal.Decimal) (target *po.MinerInfo, err error) {
	target = &po.MinerInfo{
		Epoch:                source.Epoch,
		Miner:                source.Miner.Address(),
		Owner:                source.Owner.Address(),
		Worker:               source.Worker.Address(),
		Controllers:          nil,
		RawBytePower:         source.RawBytePower,
		QualityAdjPower:      source.QualityAdjPower,
		Balance:              source.Balance,
		AvailableBalance:     source.AvailableBalance,
		VestingFunds:         source.VestingFunds,
		FeeDebt:              source.FeeDebt,
		SectorSize:           source.SectorSize,
		SectorCount:          source.SectorCount,
		FaultSectorCount:     source.FaultSectorCount,
		ActiveSectorCount:    source.ActiveSectorCount,
		LiveSectorCount:      source.LiveSectorSector,
		RecoverSectorCount:   source.RecoverSectorCount,
		TerminateSectorCount: source.TerminateSectorCount,
		PreCommitSectorCount: source.PreCommitSectorCount,
		InitialPledge:        source.InitialPledge,
		PreCommitDeposits:    source.PreCommitDeposits,
		Ips:                  nil,
	}
	for _, v := range source.ControlAddresses {
		target.Controllers = append(target.Controllers, chain.SmartAddress(v).Address())
	}

	for _, v := range source.Multiaddrs {
		var maddr multiaddr.Multiaddr
		var d []byte
		d, err = base64.StdEncoding.DecodeString(v.Data)
		if err != nil {
			return
		}
		maddr, err = multiaddr.NewMultiaddrBytes(d)
		if err != nil {
			return
		}
		target.Ips = append(target.Ips, maddr.String())
	}
	if totalPower.GreaterThan(decimal.Zero) {
		target.QualityAdjPowerPercent = target.QualityAdjPower.Div(totalPower)
	}

	return
}
