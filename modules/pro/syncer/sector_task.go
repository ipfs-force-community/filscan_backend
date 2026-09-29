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
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain/upgrader/message_detail"
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

	// NV29（Solstice / FIP-0118）之后链上换用 v19 的扇区算力口径：带 FULL_QA_POWER 标志的
	// 扇区恒为 10x，且 QA 周期起点从 Activation 变成 PowerBaseEpoch。历史数据不重算，
	// 只有 epoch >= UpgradeSolsticeHeight 才走新口径。
	nv29 := ctx.Epoch() >= message_detail.UpgradeSolsticeHeight

	var sectors []*propo.MinerSector

	sectorsMap := map[int64]*propo.MinerSector{}
	sectorSize := decimal.NewFromInt(info.SectorSize)
	for _, v := range r.SectorExpirations {
		hour := v.Expiration / 120 * 120
		vv := s.prepareSector(hour, v, sectorSize, nv29)
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

	// 三桶按占比切分，和恒等于自算的 QA 总量。
	totalQA := totalVDC.Add(totalDC).Add(totalCC)

	// 对账：只告警、不中断高度。
	//
	// 老代码是拿 londobell 用同一个（错的）v11 公式算出来的 VDC/DC/CC 跟本地求和互比，
	// 两边同错所以恒等成立、永远查不出问题；这里换成与链上真值对账：
	//   1. 自算 QA 总量（扇区求和） vs 链上 info.QualityAdjPower；
	//   2. 自算原始算力 vs 链上 info.RawBytePower；
	//   3. londobell 返回的三桶 vs 本地三桶（用于发现两侧口径版本不一致）。
	// 校验失败不再 return err：口径/数据质量差异不该把整个高度打成失败重试。
	if !powerNearEqual(totalQA, info.QualityAdjPower) {
		ctx.Warnf("miner %s epoch %s QA 总量与链上不一致: 扇区求和=%s 链上 QualityAdjPower=%s diff=%s nv29=%v",
			info.Miner.Address(), ctx.Epoch(), totalQA, info.QualityAdjPower, totalQA.Sub(info.QualityAdjPower), nv29)
	}

	if !powerNearEqual(totalRawBytePower, info.RawBytePower) {
		ctx.Warnf("miner %s epoch %s 原始算力与链上不一致: 扇区求和=%s 链上 RawBytePower=%s",
			info.Miner.Address(), ctx.Epoch(), totalRawBytePower, info.RawBytePower)
	}

	if !powerNearEqual(r.VDCPower, totalVDC) || !powerNearEqual(r.DCPower, totalDC) || !powerNearEqual(r.CCPower, totalCC) {
		ctx.Warnf("miner %s epoch %s londobell 三桶与本地口径不一致: londobell(vdc=%s dc=%s cc=%s) 本地(vdc=%s dc=%s cc=%s)",
			info.Miner.Address(), ctx.Epoch(), r.VDCPower, r.DCPower, r.CCPower, totalVDC, totalDC, totalCC)
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
		// 落库用本地按统一口径（QASplit）自己算出来的三桶，不依赖对端 londobell 的版本。
		VdcPower: totalVDC,
		DcPower:  totalDC,
		CCPower:  totalCC,
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

// powerNearEqual 比较两个算力值是否「一致」，允许 1ppm 的相对误差：
// decimal 除法在 16 位有效数字处截断，逐扇区按占比切分再求和的场景下会有极小的舍入漂移。
func powerNearEqual(a, b decimal.Decimal) bool {
	if a.Equal(b) {
		return true
	}
	diff := a.Sub(b).Abs()
	max := a.Abs()
	if b.Abs().GreaterThan(max) {
		max = b.Abs()
	}
	return diff.Mul(decimal.NewFromInt(1_000_000)).LessThanOrEqual(max)
}

func (s SectorTask) prepareSector(hour int64, v *londobell.MinerSector, size decimal.Decimal, nv29 bool) (item *propo.MinerSector) {

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

	if nv29 {
		// NV29（Solstice / FIP-0118，calibnet 4109133）之后的 v19 口径：
		//   - 带 FULL_QA_POWER(1<<1) 标志的扇区恒为 10x（新扇区、method 37 升级的老扇区）；
		//   - QA 周期起点是 PowerBaseEpoch 而不是 Activation（续期扇区两者不同）；
		//   - vdw 的语义变成「piece 的总时空」，dw 在新扇区恒为 0；
		//   - FIP-0118 之后没有 verified deal，满 QA 扇区整块算力归容量算力。
		// 实现见 pkg/londobell/qa_split.go（与 londobell 侧 adapter/qa_split.go 同源）。
		_, vdc, dc, cc, _ := londobell.QASplit(
			uint64(size.IntPart()),
			v.Flags,
			v.PowerBaseEpoch,
			v.Activation,
			v.Expiration,
			v.DealWeight.BigInt(),
			v.VerifiedDealWeight.BigInt(),
		)
		item.Vdc = decimal.NewFromBigInt(vdc, 0)
		item.Dc = decimal.NewFromBigInt(dc, 0)
		item.Cc = decimal.NewFromBigInt(cc, 0)
		return
	}

	// epoch < UpgradeSolsticeHeight 的历史数据保持 v11 老口径不动（不回溯重算）。
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
