package dal

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
)

func NewInviteDal(db *gorm.DB) *InviteDal {
	return &InviteDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ repository.InviteCodeRepo = (*InviteDal)(nil)

type InviteDal struct {
	*_dal.BaseDal
}

func (i InviteDal) GetInviteSuccessRecord(ctx context.Context, userID int64) (bool, error) {
	db, err := i.DB(ctx)
	if err != nil {
		return false, err
	}

	res := &po.InviteSuccessRecords{}

	err = db.First(res, "user_id = ?", userID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	} else if err != nil && errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}

	return true, nil
}

func (i InviteDal) SaveSuccessRecords(ctx context.Context, userID int64) error {
	db, err := i.DB(ctx)
	if err != nil {
		return err
	}
	return db.Save(&po.InviteSuccessRecords{
		UserId:       userID,
		CompleteTime: time.Now(),
	}).Error
}

func (i InviteDal) GetUserIDByInviteCode(ctx context.Context, code string) (int, error) {
	db, err := i.DB(ctx)
	if err != nil {
		return 0, err
	}
	item := po.InviteCode{}
	err = db.First(&item, "code = ?", code).Error
	return item.UserId, err
}

func (i InviteDal) UpdateUserIsValid(ctx context.Context, userID int64) error {
	db, err := i.DB(ctx)
	if err != nil {
		return err
	}
	return db.Model(&po.UserInviteRecord{}).Where("user_id = ?", userID).Update("is_valid", true).Error
}

func (i InviteDal) GetUserInviteRecordByUserID(ctx context.Context, userID int) (item po.UserInviteRecord, err error) {
	db, err := i.DB(ctx)
	if err != nil {
		return
	}

	err = db.First(&item, "user_id = ?", userID).Error
	return item, err
}

func (i InviteDal) GetUserInviteCode(ctx context.Context, userID int) (item po.InviteCode, err error) {
	db, err := i.DB(ctx)
	if err != nil {
		return
	}

	err = db.First(&item, "user_id = ?", userID).Error
	return item, err
}

func (i InviteDal) SaveUserInviteCode(ctx context.Context, userID int, code string) (err error) {
	db, err := i.DB(ctx)
	if err != nil {
		return
	}
	return db.Save(&po.InviteCode{
		Code:   code,
		UserId: userID,
	}).Error
}

func (i InviteDal) SaveUserInviteRecord(ctx context.Context, userID int, code, email string, createAt time.Time) (err error) {
	db, err := i.DB(ctx)
	if err != nil {
		return
	}
	return db.Save(&po.UserInviteRecord{
		Code:         code,
		UserId:       userID,
		RegisterTime: createAt,
		UserEmail:    email,
		IsValid:      false,
	}).Error
}

func (i InviteDal) GetUserInviteRecordByCode(ctx context.Context, code string) (items []*po.UserInviteRecord, err error) {
	db, err := i.DB(ctx)
	if err != nil {
		return
	}

	err = db.Find(&items, "code = ?", code).Error
	return
}
