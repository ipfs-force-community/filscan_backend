package repository

import (
	"context"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

type LuckRepository interface {
	GetNetQualityAjdPowerByPoints(ctx context.Context, points []chain.Epoch) (powers []*bo.LuckQualityAdjPower, err error)
	GetMinerQualityAjdPowerByRange(ctx context.Context, miner string, start, end chain.Epoch) (powers []*bo.LuckQualityAdjPower, err error)
	GetMinerQualityAjdPowerByPoints(ctx context.Context, miner string, points []chain.Epoch) (powers []*bo.LuckQualityAdjPower, err error)
	GetNetTickets(ctx context.Context, epochs chain.LCRORange, duration int64) (items []*bo.LuckNetTicket, err error)
	GetNetTicketsYear(ctx context.Context, epochs chain.LCRORange, duration int64) (items []*bo.LuckNetTicket, err error)
	GetMinerTicketsByEpochs(ctx context.Context, miner string, epochs chain.LCRORange) (total int64, err error)
	GetMiners(ctx context.Context, epoch int64) (miners []string, err error)
}
