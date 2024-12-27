package prodal

import (
	"context"

	"github.com/pkg/errors"
	probo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/bo"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewGroupDal(db *gorm.DB) *GroupDal {
	return &GroupDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.GroupRepo = (*GroupDal)(nil)

type GroupDal struct {
	*_dal.BaseDal
}

func (g GroupDal) SelectGroupByID(ctx context.Context, ID int64) (group *propo.Group, err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	table := propo.Group{}
	err = tx.Table(table.TableName()).Where("id = ?", ID).First(&group).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			group = nil
		}
		return
	}
	return
}

func (g GroupDal) SelectGroupsByUserID(ctx context.Context, userID int64) (group []*propo.Group, err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Table(propo.Group{}.TableName()).Where("user_id = ?", userID).Order("id DESC").Find(&group).Error
	if err != nil {
		return
	}
	return
}

func (g GroupDal) SelectActiveGroupsByUserID(ctx context.Context, userID int64) (group []*propo.Group, err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`
	with a as (select group_id
	           from pro.user_miners
	           where user_id = ?
	           group by group_id
	           order by group_id desc)
	select coalesce(a.group_id, 0) as id, coalesce(b.group_name, 'default') as group_name
	from a
	         left join pro.groups b on a.group_id = b.id
    `, userID).Find(&group).Error
	if err != nil {
		return
	}
	return
}

func (g GroupDal) SelectGroupMinersByUserID(ctx context.Context, userID int64) (groups []*probo.GroupMiners, err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	sql := `
SELECT id AS group_id, group_name, ARRAY_AGG(COALESCE(gm.miner_id, '')) AS miners_id
FROM pro.groups g
         LEFT JOIN pro.user_miners gm ON g.id = gm.group_id
WHERE gm.user_id = ?
GROUP BY id
`
	err = tx.Raw(sql, userID).
		Find(&groups).Error
	if err != nil {
		return
	}
	return
}

func (g GroupDal) SelectGroupMinersByGroupID(ctx context.Context, groupID int64) (groups []*probo.GroupMiners, err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	sql := `
SELECT id AS group_id, group_name, ARRAY_AGG(COALESCE(gm.miner_id, '')) AS miners_id
FROM pro.groups g
         LEFT JOIN pro.user_miners gm ON g.id = gm.group_id
WHERE gm.group_id = ?
GROUP BY id
`
	err = tx.Raw(sql, groupID).
		Find(&groups).Error
	if err != nil {
		return
	}
	return
}

func (g GroupDal) CreateGroup(ctx context.Context, group *propo.Group) (err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Create(group).Error
	if err != nil {
		return
	}

	return
}

func (g GroupDal) UpdateGroup(ctx context.Context, group *propo.Group) (result int64, err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Model(group).Updates(*group).Error
	if err != nil {
		return
	}
	result = group.Id
	return
}

func (g GroupDal) DeleteGroup(ctx context.Context, id int64) (err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Model(propo.Group{}).Delete("id = ?", id).Error
	if err != nil {
		return
	}
	return
}

func (g GroupDal) GetUserGroupNodes(ctx context.Context, userId, groupId int64) (nodes []*propo.UserMiner, err error) {
	tx, err := g.DB(ctx)
	if err != nil {
		return
	}
	tx = tx.Where(tx.Where("user_id = ?", userId))
	if groupId > 0 {
		tx = tx.Where("group_id = ?", groupId)
	}
	err = tx.Order("updated_at desc").Find(&nodes).Error
	if err != nil {
		return
	}
	return
}

func (g GroupDal) GetUserGroupByName(ctx context.Context, userId int64, groupName string) (group *propo.Group, err error) {

	tx, err := g.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Debug().Where("user_id = ? and group_name = ?", userId, groupName).First(&group).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			group = nil
		}
		return
	}
	return
}

//func (g GroupDal) SelectMinersByMiners(ctx context.Context, miners []string) (userMiner []*propo.UserMiner, err error) {
//	tx, err := g.DB(ctx)
//	if err != nil {
//		return
//	}
//
//	return
//}
