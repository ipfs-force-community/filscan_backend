package prodal

import (
	"context"
	"fmt"
	"time"

	"github.com/pkg/errors"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	propo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/po"
	prorepo "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/repo"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func NewMemberShipDal(db *gorm.DB) *MemberShipDal {
	return &MemberShipDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ prorepo.MemberShipRepo = (*MemberShipDal)(nil)

type MemberShipDal struct {
	*_dal.BaseDal
}

func (m MemberShipDal) GetMemberShipByUserID(ctx context.Context, userID int64) (membership *propo.MemberShip, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return nil, err
	}
	table := propo.MemberShip{}
	err = tx.Table(table.TableName()).Where("user_id = ?", userID).First(&membership).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
			membership = &propo.MemberShip{
				UserId:      userID,
				MemType:     string(pro.NormalVIP),
				ExpiredTime: time.Now(),
				CreatedAt:   time.Now(),
				UpdatedAt:   time.Now(),
			}
		}
	}
	return membership, err
}

func (m MemberShipDal) GetAllUserMemberShip(ctx context.Context) (memberships []*propo.MemberShip, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return nil, err
	}
	table := propo.MemberShip{}
	err = tx.Table(table.TableName()).Find(&memberships).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			err = nil
		}
	}
	return memberships, err
}

func (m MemberShipDal) CreateUserMemberShip(ctx context.Context, membership *propo.MemberShip) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{"mem_type",
			"expired_time", "updated_at"}),
	}).CreateInBatches(membership, 5)
	err = tmp.Error
	fmt.Println(tmp.RowsAffected)
	return
}

func (m MemberShipDal) CreateRechargeRecord(ctx context.Context, record *propo.RechargeRecord) (err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	err = tx.Create(record).Error
	return
}

func (m MemberShipDal) UpdateUserVIPExpire(ctx context.Context, userID int64) (rowsAffected int64, err error) {
	tx, err := m.DB(ctx)
	if err != nil {
		return
	}
	tmp := tx.Exec(`UPDATE pro.membership SET mem_type = 'NormalVIP' WHERE  user_id = ?`, userID)
	rowsAffected = tmp.RowsAffected
	err = tmp.Error
	return
}
