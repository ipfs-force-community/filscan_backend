package mdal

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	mpo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/po"
	mrepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/repo"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewRuleDal(db *gorm.DB) *RuleDal {
	return &RuleDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ mrepo.RuleRepo = (*RuleDal)(nil)

type RuleDal struct {
	*_dal.BaseDal
}

func (r RuleDal) CreateUserRule(ctx context.Context, userRules []*mpo.Rule) (err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			//{Name: "id"},
			{Name: "user_id"},
			{Name: "monitor_type"},
			{Name: "group_id"},
			{Name: "miner_id_or_all"},
			{Name: "account_type"},
			{Name: "account_addr"},
		}, //uuid不能随便更新
		DoUpdates: clause.AssignmentColumns([]string{"operator",
			"operand", "mail_alert", "msg_alert", "call_alert", "is_active", "is_vip", "description", "updated_at"}),
	}).CreateInBatches(userRules, 100)
	err = tmp.Error
	fmt.Println(tmp.RowsAffected)
	return
}

// SelectRulesByUserIDAndType 获取所有的后，在内存中进行处理。数据量本身也不多
func (r RuleDal) SelectRulesByUserIDAndType(ctx context.Context, userID int64, typeName string) (userRules []*mpo.Rule, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Model(mpo.Rule{}).Where("user_id = ? and monitor_type = ?", userID, typeName).Find(&userRules).Order("updated_at ASC").Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
	}
	return
}

// 将group_id、miner_id 聚合，主要是balance有多个account_type，将他们的描述给合并起来。
func (r RuleDal) DeleteUUIDRule(ctx context.Context, userID int64, uuid string) (rowsAffected int64, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Model(mpo.Rule{}).Where("user_id = ? and uuid = ?", userID, uuid).Delete(&mpo.Rule{})
	rowsAffected = tmp.RowsAffected
	err = tmp.Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return
}

func (r RuleDal) DeleteGroupIDRule(ctx context.Context, userID int64, groupID int64) (rowsAffected int64, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Model(mpo.Rule{}).Where("user_id = ? and group_id = ?", userID, groupID).Delete(&mpo.Rule{})
	rowsAffected = tmp.RowsAffected
	err = tmp.Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return
}

func (r RuleDal) DeleteMinerIDRule(ctx context.Context, userID int64, minerID string) (rowsAffected int64, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Model(mpo.Rule{}).Where("user_id = ? and miner_id_or_all = ?", userID, minerID).Delete(&mpo.Rule{})
	rowsAffected = tmp.RowsAffected
	err = tmp.Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return
}

func (r RuleDal) UpdateActiveState(ctx context.Context, userID int64, uuid string) (rowsAffected int64, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	rule := mpo.Rule{}
	if err = rule.BeforeUpdate(tx); err != nil {
		return
	} //手动更新时间
	tmp := tx.Exec(`UPDATE monitor.rules SET is_active = NOT is_active , updated_at = ? WHERE UUID = ? AND user_id = ?`,
		rule.UpdatedAt, uuid, userID)
	rowsAffected = tmp.RowsAffected
	err = tmp.Error
	return
}

func (r RuleDal) UpdateUserMinerGroup(ctx context.Context, userID int64, minerID string, groupID int64) (rowsAffected int64, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Exec(`UPDATE monitor.rules SET group_id = ? WHERE user_id = ? AND miner_id_or_all = ?`,
		groupID, userID, minerID)
	rowsAffected = tmp.RowsAffected
	err = tmp.Error
	return
}

func (r RuleDal) SelectRulesByUUID(ctx context.Context, userID int64, uuid string) (userRules []*mpo.Rule, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Model(mpo.Rule{}).Where("user_id = ? and uuid = ?", userID, uuid).Find(&userRules).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return
}

func (r RuleDal) SelectMinersByUserID(ctx context.Context, userID int64) (userMiners []*propo.UserMiner, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Where("user_id = ?", userID).Find(&userMiners).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
	}
	return
}

func (r RuleDal) SelectRuleUUID(ctx context.Context, userID int64, groupID int64, miner string, mType string) (userRules *mpo.Rule, err error) {
	tx, err := r.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Model(mpo.Rule{}).Where("user_id = ? and group_id = ? and miner_id_or_all = ? and monitor_type = ?", userID, groupID, miner, mType).First(&userRules).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = nil
		userRules = nil
	}
	return
}
