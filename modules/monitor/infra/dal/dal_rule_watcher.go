package mdal

import (
	"context"

	"github.com/pkg/errors"

	mpo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/po"
	mrepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/monitor/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewRuleWatcherDal(db *gorm.DB) *RuleWatcherDal {
	return &RuleWatcherDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ mrepo.RuleWatcherRepo = (*RuleWatcherDal)(nil)

type RuleWatcherDal struct {
	*_dal.BaseDal
}

func (rw RuleWatcherDal) GetAllRule(ctx context.Context) (userRules []*mpo.Rule, err error) {
	tx, err := rw.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Model(&mpo.Rule{}).Find(&userRules, "is_active = TRUE AND is_vip = TRUE").Order("uuid DESC, account_addr ASC").Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			userRules = nil
		}
		return
	}
	return
}

func (rw RuleWatcherDal) UpdateUserRuleVIPExpire(ctx context.Context, userID int64, isVip bool) (rowsAffected int64, err error) {
	tx, err := rw.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Exec(`UPDATE monitor.rules SET is_vip = ? WHERE  user_id = ?`, isVip, userID)
	rowsAffected = tmp.RowsAffected
	err = tmp.Error
	return
}

func (rw RuleWatcherDal) SelectRulesByUserID(ctx context.Context, userID int64) (userRules []*mpo.Rule, err error) {
	tx, err := rw.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Model(mpo.Rule{}).Where("user_id = ?", userID).Find(&userRules).Order("updated_at ASC").Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
	}
	return
}

//func (r RuleDal) CreateUserRule(ctx context.Context, userRules []*mpo.Rule) (err error) {
//	tx, err := r.DB(ctx)
//	if err != nil {
//		return
//	}
//	tmp := tx.Clauses(clause.OnConflict{
//		Columns: []clause.Column{
//			//{Name: "id"},
//			{Name: "user_id"},
//			{Name: "group_id"},
//			{Name: "monitor_type"},
//			{Name: "account_type"},
//		},
//		DoUpdates: clause.AssignmentColumns([]string{"monitor_type", "operator",
//			"operand", "mail_alert", "msg_alert", "call_alert", "is_active", "is_vip", "description", "updated_at"}),
//	}).CreateInBatches(userRules, 100)
//	err = tmp.Error
//	fmt.Println(tmp.RowsAffected)
//	return
//}
