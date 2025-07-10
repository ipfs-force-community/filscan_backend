package prodal

import (
	"context"
	"log"
	"time"

	"github.com/pkg/errors"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
)

func NewAuthDal(db *gorm.DB) *AuthDal {
	return &AuthDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.AuthRepo = (*AuthDal)(nil)

type AuthDal struct {
	*_dal.BaseDal
}

func (a AuthDal) UpdateUserName(ctx context.Context, id int64, name string) (err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}

	table := propo.User{}
	err = tx.Table(table.TableName()).Where("id = ?", id).Update("name", name).Error
	if err != nil {
		return
	}
	return
}

func (a AuthDal) UpdateUserLoginTime(ctx context.Context, id int64, loginAt, lastLoginAt time.Time) (err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}

	table := propo.User{}
	err = tx.Table(table.TableName()).Where("id = ?", id).Update("login_at", loginAt).Error
	if err != nil {
		return
	}
	err = tx.Table(table.TableName()).Where("id = ?", id).Update("last_login_at", lastLoginAt).Error
	if err != nil {
		return
	}
	return
}

func (a AuthDal) GetUserById(ctx context.Context, id int64) (user *propo.User, err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}

	err = tx.Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
		return
	}

	return
}

// 一般调用此时，肯定先要有userid
func (a AuthDal) GetActivityStateAndSetTrue(ctx context.Context, id int64) (isActivity bool, err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}

	user := new(propo.User)
	err = tx.Where("id = ?", id).First(user).Error
	if err != nil {
		return
	}
	if user.IsActivity == true { // 曾经参加过活动，直接返回
		return true, nil
	}
	err = tx.Table(user.TableName()).Where("id = ?", id).Update("is_activity", true).Error
	if err != nil {
		return
	}
	return false, nil
}

func (a AuthDal) UpdateUserPassword(ctx context.Context, id int64, password string) (err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}

	table := propo.User{}
	err = tx.Table(table.TableName()).Where("id = ?", id).Update("password", password).Error
	if err != nil {
		return
	}

	return
}

func (a AuthDal) GetUserByMailOrNil(ctx context.Context, mail string) (user *propo.User, err error) {
	tx, err := a.DB(ctx)
	if err != nil {
		return
	}

	user = new(propo.User)
	err = tx.Where("mail = ?", mail).First(user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			user = nil
		}
		return
	}

	return
}

func (a AuthDal) SaveUser(ctx context.Context, user *propo.User) (err error) {

	tx, err := a.DB(ctx)
	if err != nil {
		return
	}
	tx = tx.Create(user)
	log.Println(tx.RowsAffected)
	return tx.Error
}
