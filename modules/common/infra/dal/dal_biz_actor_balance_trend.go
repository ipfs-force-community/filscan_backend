package dal

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/domain/actor"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewActorBalanceTrendBizDal(db *gorm.DB) *ActorBalanceTrendBizDal {
	return &ActorBalanceTrendBizDal{BaseDal: _dal.NewBaseDal(db)}
}

type ActorBalanceTrendBizDal struct {
	*_dal.BaseDal
}

func (m ActorBalanceTrendBizDal) GetActorBalanceTrend(ctx context.Context, actorID actor.Id, start chain.Epoch, points []chain.Epoch) (actorBalanceTrend []*bo.ActorBalanceTrend, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	var miners []*po.ActorBalance
	//err = tx.Debug().Where("actor_id = ? AND epoch >= ?", actorID, start.Int64()).
	//	Order("epoch desc").
	//	Find(&miners).Error

	// month
	//err = tx.Debug().Raw(`
	//	WITH max_epoch AS (
	//		SELECT max(epoch) as max_epoch,((epoch+ 720)/ 2880 * 2880 +2160) as dayE FROM chain.actor_balances
	//		WHERE actor_id = ? AND epoch >= ?
	//		GROUP BY dayE
	//	)
	//	SELECT b.* FROM chain.actor_balances b
	//	JOIN max_epoch me ON b.epoch = me.max_epoch
	//	WHERE actor_id = ?
	//	ORDER BY epoch DESC
	//`, actorID, start.Int64(), actorID).Find(&miners).Error

	// general
	tmp := strings.Builder{}
	tmp.WriteString("CASE\n")
	for i := 1; i < len(points); i++ {
		tmp.WriteString(fmt.Sprintf("WHEN epoch > %d AND epoch <= %d THEN %d\n", points[i-1], points[i], points[i]))
	}
	tmp.WriteString("END AS dayE")
	tmpSql := tmp.String()
	sql := fmt.Sprintf(`
		WITH max_epoch AS (
			SELECT max(epoch) as max_epoch, 
			%s
			FROM chain.actor_balances
			WHERE actor_id = ? AND epoch >= ?
			GROUP BY dayE
		)
		SELECT b.* FROM chain.actor_balances b
		JOIN max_epoch me ON b.epoch = me.max_epoch
		WHERE actor_id = ?
		ORDER BY epoch DESC
		`, tmpSql)
	err = tx.Raw(sql, actorID, start.Int64(), actorID).Find(&miners).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}

	for _, miner := range miners {
		minerBalance := &bo.ActorBalanceTrend{
			Epoch:     miner.Epoch,
			AccountID: miner.ActorId,
			Balance:   miner.Balance,
		}
		actorBalanceTrend = append(actorBalanceTrend, minerBalance)
	}
	return
}

func (m ActorBalanceTrendBizDal) GetActorUnderEpochBalance(ctx context.Context, actorID actor.Id, start chain.Epoch) (actorBalanceTrend *bo.ActorBalanceTrend, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	var miner *po.ActorBalance
	err = tx.Where("actor_id = ? AND epoch < ?", actorID, start.Int64()).
		Order("epoch desc").
		First(&miner).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}

	return &bo.ActorBalanceTrend{
		Epoch:     miner.Epoch,
		AccountID: miner.ActorId,
		Balance:   miner.Balance,
	}, nil
}

func (m ActorBalanceTrendBizDal) GetLatestEpoch(ctx context.Context) (epoch *chain.Epoch, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}

	var actorBalance po.ActorBalance
	err = tx.Model(&actorBalance).
		Distinct().
		Select("epoch").
		Order("epoch DESC").
		First(&actorBalance).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
	}
	actorEpoch := chain.Epoch(actorBalance.Epoch)
	epoch = &actorEpoch
	return
}
