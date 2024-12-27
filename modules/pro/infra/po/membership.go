package propo

import (
	"time"

	"gorm.io/gorm"
)

type MemberShip struct {
	Id          int64
	UserId      int64
	MemType     string
	ExpiredTime time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (m *MemberShip) TableName() string {
	return "pro.membership"
}

func (m *MemberShip) BeforeCreate(tx *gorm.DB) (err error) {
	m.CreatedAt = time.Now()
	m.UpdatedAt = time.Now()
	return
}

func (m *MemberShip) BeforeUpdate(tx *gorm.DB) (err error) {
	m.UpdatedAt = time.Now()
	return
}

type RechargeRecord struct {
	Id         int64
	UserId     int64
	MemType    string
	ExtendTime string
	CreatedAt  time.Time
}

func (m *RechargeRecord) TableName() string {
	return "pro.recharge_record"
}

func (m *RechargeRecord) BeforeCreate(tx *gorm.DB) (err error) {
	m.CreatedAt = time.Now()
	return
}
