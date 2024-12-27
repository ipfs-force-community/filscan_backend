package propo

import (
	"gorm.io/gorm"
	"time"
)

type Group struct {
	Id        int64
	UserId    int64
	GroupName string
	CreatedAt time.Time
	UpdatedAt time.Time
	IsDefault bool
}

func (Group) TableName() string {
	return "pro.groups"
}

func (u *Group) BeforeCreate(tx *gorm.DB) (err error) {
	u.CreatedAt = time.Now()
	u.UpdatedAt = time.Now()
	return
}

func (u *Group) BeforeUpdate(tx *gorm.DB) (err error) {
	u.UpdatedAt = time.Now()
	return
}
