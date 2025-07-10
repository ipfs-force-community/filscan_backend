package acl

import (
	"context"

	"github.com/shopspring/decimal"
	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

type AdapterStatisticAcl interface {
	CurrentSectorInitialPledge(ctx context.Context, epoch *chain.Epoch) (*londobell.CurrentSectorInitialPledge, error)
}

func NewStatisticAclImpl(adapter AdapterStatisticAcl) *StatisticAclImpl {
	return &StatisticAclImpl{adapter: adapter}
}

type StatisticAclImpl struct {
	adapter AdapterStatisticAcl
}

func (s StatisticAclImpl) GetFilCompose(ctx context.Context) (filCompose filscan.FilCompose, err error) {
	initialPledge, err := s.adapter.CurrentSectorInitialPledge(ctx, nil)
	if err != nil {
		return filscan.FilCompose{}, err
	}
	// FIL的存储奖励发放: 1,100,000,000 FIL
	totalMined := decimal.NewFromInt(11).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(8)))
	totalMinedAttoFil := totalMined.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	remainingMined := totalMinedAttoFil.Sub(initialPledge.FilMined)
	// FIL的锁仓奖励发放: 600,000,000 FIL
	totalVested := decimal.NewFromInt(6).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(8)))
	totalVestedAttoFil := totalVested.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	remainingVested := totalVestedAttoFil.Sub(initialPledge.FilVested)
	// FIL的保留部分: 300,000,000 FIL
	totalReserved := decimal.NewFromInt(3).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(8)))
	totalReservedAttoFil := totalReserved.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	remainingVReserved := totalReservedAttoFil.Sub(initialPledge.FilReserveDisbursed)
	// 全部已释放的FIL
	totalReleased := initialPledge.FilLocked.Add(initialPledge.FilBurnt).Add(initialPledge.FilCirculating)
	filCompose = filscan.FilCompose{
		Mined:             initialPledge.FilMined,
		RemainingMined:    remainingMined,
		Vested:            initialPledge.FilVested,
		RemainingVested:   remainingVested,
		ReserveDisbursed:  initialPledge.FilReserveDisbursed,
		RemainingReserved: remainingVReserved,
		Locked:            initialPledge.FilLocked,
		Burnt:             initialPledge.FilBurnt,
		Circulating:       initialPledge.FilCirculating,
		TotalReleased:     totalReleased,
	}
	return
}

func (s StatisticAclImpl) GetFilComposeByEpoch(ctx context.Context, epoch *chain.Epoch) (filCompose filscan.FilCompose, err error) {
	initialPledge, err := s.adapter.CurrentSectorInitialPledge(ctx, epoch)
	if err != nil {
		return filscan.FilCompose{}, err
	}
	// FIL的存储奖励发放: 1,100,000,000 FIL
	totalMined := decimal.NewFromInt(11).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(8)))
	totalMinedAttoFil := totalMined.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	remainingMined := totalMinedAttoFil.Sub(initialPledge.FilMined)
	// FIL的锁仓奖励发放: 600,000,000 FIL
	totalVested := decimal.NewFromInt(6).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(8)))
	totalVestedAttoFil := totalVested.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	remainingVested := totalVestedAttoFil.Sub(initialPledge.FilVested)
	// FIL的保留部分: 300,000,000 FIL
	totalReserved := decimal.NewFromInt(3).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(8)))
	totalReservedAttoFil := totalReserved.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))
	remainingVReserved := totalReservedAttoFil.Sub(initialPledge.FilReserveDisbursed)
	// 全部已释放的FIL
	totalReleased := initialPledge.FilLocked.Add(initialPledge.FilBurnt).Add(initialPledge.FilCirculating)
	filCompose = filscan.FilCompose{
		Mined:             initialPledge.FilMined,
		RemainingMined:    remainingMined,
		Vested:            initialPledge.FilVested,
		RemainingVested:   remainingVested,
		ReserveDisbursed:  initialPledge.FilReserveDisbursed,
		RemainingReserved: remainingVReserved,
		Locked:            initialPledge.FilLocked,
		Burnt:             initialPledge.FilBurnt,
		Circulating:       initialPledge.FilCirculating,
		TotalReleased:     totalReleased,
	}
	return
}
