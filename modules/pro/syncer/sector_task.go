package prosyncer

import (
	"context"
	"fmt"
	"time"

	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/big"
	sminer "github.com/filecoin-project/go-state-types/builtin/v11/miner"
	"github.com/gozelle/async/parallel"
	"github.com/shopspring/decimal"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewSectorTask(db *gorm.DB, store bool) *SectorTask {
	return &SectorTask{saver: newSaver(db), db: db, store: store}
}

var _ syncer.Task = (*SectorTask)(nil)

type SectorTask struct {
	db    *gorm.DB
	saver iSaver
	store bool
}

func (s SectorTask) Name() string {
	return "sector-task"
}

func (s SectorTask) RollBack(ctx context.Context, gteEpoch chain.Epoch) (err error) {
	err = s.saver.RollbackMinerSectors(ctx, gteEpoch)
	if err != nil {
		return
	}
	return
}

func (s SectorTask) HistoryClear(ctx context.Context, safeClearEpoch chain.Epoch) (err error) {
	return
}

func (s SectorTask) Exec(ctx *syncer.Context) (err error) {

	if ctx.Epoch() != ctx.Epoch().CurrentDay() {
		return
	}

	ctx.Debugf("开始同步扇区")

	r, err := ctx.Agg().MinersInfo(ctx.Context(), ctx.Epoch(), ctx.Epoch().Next())
	if err != nil {
		return
	}

	total := len(r)
	if total == 0 {
		return fmt.Errorf("没有同步到miner！")
	}
	ctx.Debugf("开始同步 Sector ,总共 %d 个 miner", total)
	var runners []parallel.Runner[parallel.Null]
	for _, v := range r {
		info := v
		runners = append(runners, func(_ context.Context) (parallel.Null, error) {
			e := s.syncMinerInfosSectors(ctx, info)
			if e != nil {
				return nil, e
			}
			return nil, nil
		})
	}

	n := 0
	now := time.Now()
	ch := parallel.Run[parallel.Null](ctx.Context(), 5, runners)
	err = parallel.Wait[parallel.Null](ch, func(_ parallel.Null) error {
		n++
		if n%500 == 0 {
			ctx.Debugf("已查询 %d 个 Miner ActiveSectors, 还剩: %d 已耗时: %s", n, total-n, time.Since(now))
		}
		return nil
	})
	if err != nil {
		return
	}

	count, err := s.saver.CountMinerDcs(ctx.Context(), ctx.Epoch().Int64())
	if err != nil {
		return
	}
	if count > 0 {
		err = s.saver.DeleteMinerSectorsBeforeEpoch(ctx.Context(), ctx.Epoch())
		if err != nil {
			return
		}
	}

	return
}

func (s SectorTask) syncMinerInfosSectors(ctx *syncer.Context, info *londobell.MinerInfo) (err error) {

	exist, err := s.saver.HasMinerDc(context.Background(), info.Miner.Address(), ctx.Epoch())
	if err != nil {
		return
	}
	if exist {
		return
	}

	defer func() {
		if err != nil {
			err = fmt.Errorf("prepare miner: %s active sectors error: %s", info.Miner, err)
		}
	}()

	r, err := ctx.Adapter().ActiveSectors(ctx.Context(), info.Miner, ctx.Epoch())
	if err != nil {
		return
	}

	if r == nil {
		err = fmt.Errorf("request miner: %s epoch: %s active sectors is nil", info.Miner, ctx.Epoch())
		return
	}

	var totalVDC decimal.Decimal
	var totalDC decimal.Decimal
	var totalCC decimal.Decimal
	var totalRawBytePower decimal.Decimal
	var totalPledge decimal.Decimal

	var sectors []*propo.MinerSector

	sectorsMap := map[int64]*propo.MinerSector{}
	sectorSize := decimal.NewFromInt(info.SectorSize)
	for _, v := range r.SectorExpirations {
		hour := v.Expiration / 120 * 120
		vv := s.prepareSector(hour, v, sectorSize)
		if _, ok := sectorsMap[hour]; !ok {
			item := &propo.MinerSector{
				Epoch:     ctx.Epoch().Int64(),
				Miner:     info.Miner.Address(),
				HourEpoch: hour,
			}
			sectorsMap[hour] = item
			sectors = append(sectors, item)
		}
		sectorsMap[hour].Sectors++
		sectorsMap[hour].Pledge = sectorsMap[hour].Pledge.Add(vv.Pledge)
		sectorsMap[hour].Power = sectorsMap[hour].Power.Add(sectorSize)
		sectorsMap[hour].Vdc = sectorsMap[hour].Vdc.Add(vv.Vdc)
		sectorsMap[hour].Dc = sectorsMap[hour].Dc.Add(vv.Dc)
		sectorsMap[hour].Cc = sectorsMap[hour].Cc.Add(vv.Cc)
	}

	for _, v := range sectors {
		totalVDC = totalVDC.Add(v.Vdc)
		totalDC = totalDC.Add(v.Dc)
		totalCC = totalCC.Add(v.Cc)
		totalPledge = totalPledge.Add(v.Pledge)
		totalRawBytePower = totalRawBytePower.Add(v.Power)
	}

	if !info.RawBytePower.Equal(totalRawBytePower) {
		err = fmt.Errorf("raw power not equal: %s != %s", info.RawBytePower, totalRawBytePower)
		return
	}

	if !r.VDCPower.Equal(totalVDC) {
		err = fmt.Errorf("vdc not equal: %s != %s", r.VDCPower, totalVDC)
		return
	}

	if !r.DCPower.Equal(totalDC) {
		err = fmt.Errorf("dc not equal: %s != %s", r.DCPower, totalDC)
		return
	}

	if !r.CCPower.Equal(totalCC) {
		err = fmt.Errorf("cc not equal: %s != %s", r.CCPower, totalCC)
		return
	}

	dc := &propo.MinerDc{
		Epoch:           ctx.Epoch().Int64(),
		Miner:           info.Miner.Address(),
		RawBytePower:    info.RawBytePower,
		QualityAdjPower: info.QualityAdjPower,
		Pledge:          info.InitialPledge,
		LiveSectors:     info.LiveSectorSector,
		ActiveSectors:   info.ActiveSectorCount,
		FaultSectors:    info.FaultSectorCount,
		SectorSize:      info.SectorSize,
		VdcPower:        r.VDCPower,
		DcPower:         r.DCPower,
		CCPower:         r.CCPower,
	}

	if !s.store {
		ctx.Infof("忽略保存 miner: %s", info.Miner.Address())
		return
	} else {
		err = s.save(dc, sectors)
		if err != nil {
			return
		}
	}

	return
}

func (s SectorTask) save(dc *propo.MinerDc, sectors []*propo.MinerSector) (err error) {

	tx := s.db.Begin()
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	cctx := _dal.ContextWithDB(context.Background(), tx)

	err = s.saver.SaveMinerSectors(cctx, sectors)
	if err != nil {
		return
	}

	err = s.saver.SaveMineDc(cctx, dc)
	if err != nil {
		return
	}

	err = tx.Commit().Error
	if err != nil {
		return
	}

	return
}

func (s SectorTask) prepareSector(hour int64, v *londobell.MinerSector, size decimal.Decimal) (item *propo.MinerSector) {

	item = &propo.MinerSector{
		Epoch:     0,
		Miner:     "",
		Sectors:   0,
		HourEpoch: hour,
		Pledge:    v.InitialPledge,
		Power:     decimal.Decimal{},
		Vdc:       decimal.Decimal{},
		Dc:        decimal.Decimal{},
		Cc:        decimal.Decimal{},
	}

	//quality := ((size*duration-(dealweight+verifiedweight)*10 + dealweight*10 + verifiedweight*100 ) << 20 ) / size*duration / 10
	//adjpower := quality * size >> 20

	duration := decimal.NewFromInt(v.Expiration - v.Activation)
	ten := decimal.NewFromInt(10)

	info := &sminer.SectorOnChainInfo{
		Activation:         abi.ChainEpoch(v.Activation),
		Expiration:         abi.ChainEpoch(v.Expiration),
		DealWeight:         big.NewFromGo(v.DealWeight.BigInt()),
		VerifiedDealWeight: big.NewFromGo(v.VerifiedDealWeight.BigInt()),
		InitialPledge:      big.NewFromGo(v.InitialPledge.BigInt()),
	}
	adjPower := decimal.NewFromBigInt(sminer.QAPowerForSector(abi.SectorSize(size.IntPart()), info).Int, 0)

	raw := duration.Mul(size)
	vdc := v.VerifiedDealWeight.Mul(ten)
	dc := v.DealWeight
	cc := raw.Sub(v.VerifiedDealWeight).Sub(v.DealWeight)
	all := vdc.Add(dc).Add(cc)

	item.Vdc = adjPower.Mul(vdc.Div(all))
	item.Dc = adjPower.Mul(dc.Div(all))
	item.Cc = adjPower.Mul(cc.Div(all))

	return
}
