package dal

import (
	"context"
	"fmt"

	"github.com/filecoin-project/go-state-types/builtin"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

var _ repository.LuckRepository = (*LuckDal)(nil)

type LuckDal struct {
	*_dal.BaseDal
}

func NewLuckDal(db *gorm.DB) *LuckDal {
	return &LuckDal{BaseDal: _dal.NewBaseDal(db)}
}

func (l LuckDal) GetNetQualityAjdPowerByPoints(ctx context.Context, points []chain.Epoch) (powers []*bo.LuckQualityAdjPower, err error) {

	tx, err := l.DB(ctx)
	if err != nil {
		return
	}

	var values []int64
	for _, v := range points {
		values = append(values, v.Int64())
	}

	err = tx.Raw(`select epoch, (state ->> 'ThisEpochQualityAdjPower')::decimal as quality_adj_power
		from chain.builtin_actor_states
		where epoch in (?)
		  and actor = ?
		order by epoch`, values, builtin.StoragePowerActorAddr.String()).Find(&powers).Error
	if err != nil {
		return
	}

	return
}

func (l LuckDal) GetMinerQualityAjdPowerByPoints(ctx context.Context, miner string, points []chain.Epoch) (powers []*bo.LuckQualityAdjPower, err error) {
	tx, err := l.DB(ctx)
	if err != nil {
		return
	}

	var values []int64
	for _, v := range points {
		values = append(values, v.Int64())
	}

	err = tx.Raw(`
		select epoch, quality_adj_power
		from chain.miner_infos
		where epoch in ?
		  and miner = ?
		order by epoch;`, values, miner).Find(&powers).Error
	if err != nil {
		return
	}

	return
}

func (l LuckDal) GetMinerQualityAjdPowerByRange(ctx context.Context, miner string, start, end chain.Epoch) (powers []*bo.LuckQualityAdjPower, err error) {
	tx, err := l.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
		select epoch, quality_adj_power
		from chain.miner_infos
		where epoch >= ? and epoch <= ?
		  and miner = ?
		order by epoch;`, start, end, miner).Find(&powers).Error
	if err != nil {
		return
	}

	return
}

func (l LuckDal) GetMinerTicketsByEpochs(ctx context.Context, miner string, epochs chain.LCRORange) (total int64, err error) {
	tx, err := l.DB(ctx)
	if err != nil {
		return
	}

	sql := `select greatest(sum(win_count),0)
		from chain.miner_win_counts
		where epoch < ?
		  and epoch >= ?
		  and miner = ?;`

	err = tx.Raw(sql, epochs.LtEnd.Int64(), epochs.GteBegin.Int64(), miner).Scan(&total).Error
	if err != nil {
		return
	}

	return
}

func (l LuckDal) GetNetTickets(ctx context.Context, epochs chain.LCRORange, duration int64) (items []*bo.LuckNetTicket, err error) {
	tx, err := l.DB(ctx)
	if err != nil {
		return
	}

	sql := fmt.Sprintf(`select %s as height, sum(win_count) as win_counts
		from chain.miner_win_counts
		where epoch < ?
		  and epoch >= ?
		group by height
		order by height desc;`,
		fmt.Sprintf("(epoch / %d * %d)", duration, duration),
	)

	err = tx.Raw(sql, epochs.LtEnd.Int64(), epochs.GteBegin.Int64()).Find(&items).Error
	if err != nil {
		return
	}

	return
}

func (l LuckDal) GetNetTicketsYear(ctx context.Context, epochs chain.LCRORange, duration int64) (items []*bo.LuckNetTicket, err error) {
	tx, err := l.DB(ctx)
	if err != nil {
		return
	}

	sql := fmt.Sprintf(`select %s as height, sum(win_count) as win_counts
		from chain.miner_win_counts
		where epoch < ?
		  and epoch >= ?
		group by height
		order by height asc;`,
		//fmt.Sprintf("((epoch+720) / %d * %d  + 2160)", duration, duration), //一天的硬编码
		fmt.Sprintf("((epoch+9360)/14400*14400 + 5040)"), // 五天的硬编码
	)

	err = tx.Raw(sql, epochs.LtEnd.Int64(), epochs.GteBegin.Int64()).Find(&items).Error
	if err != nil {
		return
	}

	return
}

func (l LuckDal) GetMiners(ctx context.Context, epoch int64) (miners []string, err error) {
	tx, err := l.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Raw(`select miner from chain.miner_infos where epoch=?;`, epoch).Scan(&miners).Error
	if err != nil {
		return
	}

	return
}
