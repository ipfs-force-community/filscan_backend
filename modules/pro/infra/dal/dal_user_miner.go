package prodal

import (
	"context"

	"github.com/pkg/errors"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	probo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/bo"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewUserMinerDal(db *gorm.DB) *UserMinerDal {
	return &UserMinerDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.UserMinerRepo = (*UserMinerDal)(nil)

type UserMinerDal struct {
	*_dal.BaseDal
}

func (u UserMinerDal) SelectGroupMinersByUserID(ctx context.Context, userID int64, limit int64) (userMiner []*probo.UserMiner, err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}

	sql := `
SELECT um.user_id, group_id, g.group_name, g.is_default, miner_id, miner_tag
FROM pro.user_miners um
         LEFT JOIN pro.groups g ON g.id = um.group_id
WHERE um.user_id = ?
ORDER BY um.miner_id
LIMIT ?
`

	err = tx.Raw(sql, userID, limit).
		Find(&userMiner).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	return
}

func (u UserMinerDal) SelectGroupMinersByGroupID(ctx context.Context, userId, groupID int64) (userMiner []*probo.UserMiner, err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}
	var sql string
	if groupID > 0 {
		sql = `
		SELECT um.user_id, group_id, g.group_name, g.is_default, miner_id, miner_tag
		FROM pro.user_miners um
		         LEFT JOIN pro.groups g ON g.id = um.group_id
		WHERE um.user_id = ? and um.group_id = ?
		ORDER BY um.updated_at
		`
		err = tx.Raw(sql, userId, groupID).
			Find(&userMiner).Error
		if err != nil {
			return
		}
	} else {
		sql = `
		SELECT um.user_id, group_id, g.group_name, g.is_default, miner_id, miner_tag
		FROM pro.user_miners um
		         LEFT JOIN pro.groups g ON g.id = um.group_id
		WHERE um.user_id = ? and um.group_id is null
		ORDER BY um.updated_at
		`
		err = tx.Raw(sql, userId).
			Find(&userMiner).Error
		if err != nil {
			return
		}
	}

	return
}

func (u UserMinerDal) SelectMinersByUserID(ctx context.Context, userID int64) (userMiner []*propo.UserMiner, err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Model(propo.UserMiner{}).Where("user_id = ?", userID).Find(&userMiner).Order("miner_id ASC").Error
	if err != nil {
		return
	}
	return
}

func (u UserMinerDal) SelectMinersByMiners(ctx context.Context, userID int64, miners []chain.SmartAddress) (userMiner []*propo.UserMiner, err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Model(propo.UserMiner{}).Where("user_id = ? and miner_id in ?", userID, miners).Find(&userMiner).Order("updated_at ASC").Error
	if err != nil {
		return
	}
	return
}

func (u UserMinerDal) CreateUserMiner(ctx context.Context, UserMiner []*propo.UserMiner) (result []*pro.MinerInfo, err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "miner_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"miner_tag", "group_id", "updated_at"}),
	}).CreateInBatches(UserMiner, 10).Error
	if err != nil {
		return
	}
	for _, miner := range UserMiner {
		result = append(result, &pro.MinerInfo{
			MinerID:  miner.MinerID,
			MinerTag: miner.MinerTag,
		})
	}

	return
}

func (u UserMinerDal) DeleteUserMinerList(ctx context.Context, groupID int64, minerIDList []chain.SmartAddress) (result []chain.SmartAddress, err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Model(propo.UserMiner{}).Where("group_id = ? AND miner_id in ?", groupID, minerIDList).Delete(&propo.UserMiner{}).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	result = minerIDList
	return
}

func (u UserMinerDal) DeleteUserMinerByGroupID(ctx context.Context, userId int64, groupID int64) (err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}

	if groupID > 0 {
		err = tx.Model(propo.UserMiner{}).Where("user_id = ?  and group_id = ?", userId, groupID).Delete(&propo.UserMiner{}).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = nil
			}
			return
		}
	} else {
		err = tx.Model(propo.UserMiner{}).Where("user_id = ? and group_id is null", userId).Delete(&propo.UserMiner{}).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				err = nil
			}
			return
		}
	}

	return
}

func (u UserMinerDal) DeleteUserMinerByMinerID(ctx context.Context, userID int64, minerID string) (err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Model(propo.UserMiner{}).Where("user_id = ?  and miner_id = ?", userID, minerID).Delete(&propo.UserMiner{}).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}
	return
}

func (u UserMinerDal) CountUserMiners(ctx context.Context, userId int64) (count int64, err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Raw(`select count(1) from pro.user_miners where user_id = ?`, userId).Scan(&count).Error
	if err != nil {
		return
	}
	return
}

func (u UserMinerDal) QueryExistsMiners(ctx context.Context, userId int64, groupId int64, miners []string) (result map[string]struct{}, err error) {

	tx, err := u.DB(ctx)
	if err != nil {
		return
	}

	var items []*propo.UserMiner
	tx = tx.Where("user_id = ? and miner_id in ?", userId, miners)
	if groupId > 0 {
		tx = tx.Where("group_id != ? or group_id is null", groupId)
	} else {
		tx = tx.Where("group_id is not null")
	}

	err = tx.Debug().Find(&items).Error
	if err != nil {
		return
	}

	result = map[string]struct{}{}
	for _, v := range items {
		result[v.MinerID.Address()] = struct{}{}
	}

	return
}
