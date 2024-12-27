package luck

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type params struct {
	start           chain.Epoch
	end             chain.Epoch
	interval        int64
	points          []chain.Epoch
	netWinCountsMap map[int64]int64
	miners          []string
	netPowersMap    map[int64]*bo.LuckQualityAdjPower
}

type Export struct {
	Epochs chain.LCRORange
	Mt     int64
	Et     decimal.Decimal
	Rows   Rows
	Luck   decimal.Decimal
}

type Row struct {
	Epoch int64
	Mpi   decimal.Decimal
	Tpi   decimal.Decimal
	Tti   int64
	Eti   decimal.Decimal
}

var _ sort.Interface = (*Rows)(nil)

type Rows []*Row

func (r Rows) Len() int {
	return len(r)
}

func (r Rows) Less(i, j int) bool {
	return r[i].Epoch > r[j].Epoch
}

func (r Rows) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}

func NewCalculator(repo repository.LuckRepository) *Calculator {
	return &Calculator{repo: repo}
}

type Calculator struct {
	repo repository.LuckRepository
}

func (c Calculator) prepareParams(ctx context.Context, end chain.Epoch, typ string) (p params, err error) {

	p.end = end

	switch typ {
	case "24h":
		p.start = end - 2880
		p.interval = 120
	case "7d":
		p.start = end - 7*2880
		p.interval = 120
	case "30d":
		p.start = end - 30*2880
		p.interval = 120
	case "1y":
		//p.end = end/2880*2880 - 720
		p.end = end
		p.start = p.end - 365*2880
		p.interval = 2880 * 5 // 5天取一个点
	default:
		err = fmt.Errorf("unsupport cacl luck rate type: %s", typ)
		return
	}

	var netWinCounts []*bo.LuckNetTicket

	if typ == "1y" {
		netWinCounts, err = c.repo.GetNetTicketsYear(ctx, chain.NewLCRORange(p.start, p.end), p.interval)
	} else {
		netWinCounts, err = c.repo.GetNetTickets(ctx, chain.NewLCRORange(p.start, p.end), p.interval)
	}
	if err != nil {
		return
	}
	p.netWinCountsMap = map[int64]int64{}
	for _, v := range netWinCounts {
		p.netWinCountsMap[v.Height] = v.WinCounts
		p.points = append(p.points, chain.Epoch(v.Height))
	}

	//for i := p.start; i < p.end; i += chain.Epoch(p.interval) {
	//	p.points = append(p.points, i)
	//}

	netPowers, err := c.repo.GetNetQualityAjdPowerByPoints(ctx, p.points)
	if err != nil {
		return
	}
	p.netPowersMap = map[int64]*bo.LuckQualityAdjPower{}
	for _, v := range netPowers {
		p.netPowersMap[v.Epoch] = v
	}
	//若这部分有节点没命中，去寻找周围的节点
	var unhit []chain.Epoch
	if len(netPowers) != len(p.points) {
		for _, point := range p.points {
			if _, ok := p.netPowersMap[point.Int64()]; !ok {
				unhit = append(unhit, point-1)
				unhit = append(unhit, point+1)
			}
		}
		netPowers, err = c.repo.GetNetQualityAjdPowerByPoints(ctx, unhit)
		if err != nil {
			return
		}
		for _, v := range netPowers {
			if v.Epoch%10 == 1 {
				p.netPowersMap[v.Epoch-1] = v
			} else {
				p.netPowersMap[v.Epoch+1] = v
			}
		}
	}
	p.miners, err = c.repo.GetMiners(ctx, end.Int64())
	if err != nil {
		return
	}

	return
}

// CalcMinersLuckRate CalcMinerLuckRate 计算 Miner 的幸运值
// 根据公式，计算 Miner 过去 24h 的幸运值，安装专利书上所说，将 24h 按 30min，划分为 48 个区间，分别计算每个区间的幸运值，求和平均得到 b, 再用 24h 内的总的赢票数 a 除以 b。
// 但是本系统中，miner 的数据都是 1h 获取一次，故最多只能分为 24h 段
// 24h,7d，<30d 的区间按小时分区间
// >=30d，按天划分区间
func (c Calculator) CalcMinersLuckRate(ctx context.Context, end chain.Epoch, typ string) (lucks map[string]decimal.Decimal, err error) {

	now := time.Now()
	defer func() {
		fmt.Printf("interval: %s 总耗时: %s\n", typ, time.Since(now))
	}()

	p, err := c.prepareParams(ctx, end, typ)
	if err != nil {
		return
	}

	lucks = map[string]decimal.Decimal{}
	for i, miner := range p.miners {
		n := time.Now()
		var luck decimal.Decimal
		luck, _, err = c.calcMinerLuckRate(ctx, miner, p, false)
		if err != nil {
			return
		}
		lucks[miner] = luck
		fmt.Printf("interval: %s mienr:%s lucky:%s 共 %d, 还剩: %d, 耗时: %s\n", typ, miner, luck.String(), len(p.miners), len(p.miners)-1-i, time.Since(n))
	}

	return
}

func (c Calculator) Export(ctx context.Context, miner string, end chain.Epoch, typ string) (export *Export, err error) {

	p, err := c.prepareParams(ctx, end, typ)
	if err != nil {
		return
	}

	_, export, err = c.calcMinerLuckRate(ctx, miner, p, true)
	if err != nil {
		return
	}

	return
}

func (c Calculator) calcMinerLuckRate(ctx context.Context, miner string, p params, trace bool) (luck decimal.Decimal, export *Export, err error) {
	minerPowers, err := c.repo.GetMinerQualityAjdPowerByPoints(ctx, miner, p.points)
	if err != nil {
		return
	}
	minerPowersMap := map[int64]*bo.LuckQualityAdjPower{}
	for _, v := range minerPowers {
		minerPowersMap[v.Epoch] = v
	}

	if trace {
		export = &Export{
			Epochs: chain.NewLCRORange(p.start, p.end),
			Luck:   decimal.Zero,
			Mt:     0,
			Et:     decimal.Decimal{},
			Rows:   nil,
		}
	}

	et := decimal.Decimal{}
	for point, tti := range p.netWinCountsMap {
		mpi, ok := minerPowersMap[point]
		if !ok {
			mpi = &bo.LuckQualityAdjPower{}
		}

		tpi, ok := p.netPowersMap[point]
		if !ok {
			tpi = &bo.LuckQualityAdjPower{}
		}

		var eti decimal.Decimal
		if tpi.QualityAdjPower.GreaterThan(decimal.Zero) {
			eti = mpi.QualityAdjPower.Mul(decimal.NewFromInt(tti)).Div(tpi.QualityAdjPower)
		}

		et = et.Add(eti)

		if export != nil {
			export.Rows = append(export.Rows, &Row{
				Epoch: point,
				Mpi:   mpi.QualityAdjPower,
				Tpi:   tpi.QualityAdjPower,
				Tti:   tti,
				Eti:   eti,
			})
		}
	}

	if et.Equal(decimal.Zero) {
		return
	}
	minerWinCounts, err := c.repo.GetMinerTicketsByEpochs(ctx, miner, chain.NewLCRORange(p.start, p.end))
	if err != nil {
		return
	}

	luck = decimal.NewFromInt(minerWinCounts).Div(et)

	if export != nil {
		export.Luck = luck
		export.Mt = minerWinCounts
		export.Et = et
		sort.Sort(export.Rows)
	}

	return
}
