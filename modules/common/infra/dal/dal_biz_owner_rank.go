package dal

import (
	"context"
	"github.com/gozelle/pongo2"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"

	filscan "gitlab.forceup.in/fil-data-factory/filscan-backend/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/bo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewOwnerRankBizDal(db *gorm.DB) *OwnerRankBizDal {
	return &OwnerRankBizDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.OwnerRankBizRepo = (*OwnerRankBizDal)(nil)

type OwnerRankBizDal struct {
	*_dal.BaseDal
}

func (o OwnerRankBizDal) GetOwnerRanks(ctx context.Context, epoch chain.Epoch, query filscan.PagingQuery) (items []*bo.OwnerRank, total int64, err error) {

	tx, err := o.DB(ctx)
	if err != nil {
		return
	}

	field := "quality_adj_power"
	order := "desc"

	join := "left"
	if query.Order != nil {
		field, order, err = resolveOrder(query.Order.Field, query.Order.Sort, ownerRankOrderColumns, field, order)
		if err != nil {
			return
		}
		// 以下字段取自 chain.owner_stats（join 侧），需要 right join 才不会丢行。
		switch query.Order.Field {
		case "rewards_ratio_24h", "power_change_24h", "block_count":
			join = "right"
		}
	}

	tpl, err := pongo2.FromString(`
	SELECT a.epoch, a.owner, a.quality_adj_power, b.acc_reward, b.acc_block_count, b.quality_adj_power_change, b.reward_power_ratio
	FROM "chain"."owner_infos" a
	         {{ JoinDirection }} join chain.owner_stats b on a.epoch = b.epoch and a.owner = b.owner 
	WHERE a.epoch = ?
	AND b.interval = '24h'
	ORDER BY {{ SortField }} {{ SortOrder }}
	OFFSET ?
	LIMIT ?;
	`)

	if err != nil {
		return
	}

	sql, err := tpl.Execute(map[string]any{
		"SortField":     field,
		"SortOrder":     order,
		"JoinDirection": join,
	})
	if err != nil {
		return
	}

	err = tx.Raw(sql, epoch.Int64(), (query.Index)*query.Limit, query.Limit).Find(&items).Error
	if err != nil {
		return
	}

	// 统计总量
	tx, err = o.DB(ctx)
	if err != nil {
		return
	}

	st := po.SyncMinerEpochPo{}
	err = tx.Select("owners").Where("epoch=?", epoch.Int64()).First(&st).Error
	if err != nil {
		return
	}
	total = st.Owners

	return
}
