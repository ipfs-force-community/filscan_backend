package po

import "time"

type InviteCode struct {
	ID     int64  `gorm:"column:id"`
	Code   string `gorm:"column:code"`
	UserId int    `gorm:"column:user_id"`
}

func (InviteCode) TableName() string {
	return "public.invite_code"
}

type UserInviteRecord struct {
	ID           int64     `gorm:"column:id"`
	Code         string    `gorm:"column:code"`
	UserId       int       `gorm:"column:user_id"`
	UserEmail    string    `gorm:"column:user_email"`
	RegisterTime time.Time `gorm:"column:register_time"`
	IsValid      bool      `gorm:"column:is_valid"`
}

func (UserInviteRecord) TableName() string {
	return "public.user_invite_record"
}

type InviteSuccessRecords struct {
	ID           int64     `gorm:"column:id"`
	UserId       int64     `gorm:"column:user_id"`
	CompleteTime time.Time `gorm:"column:complete_time"`
}

func (InviteSuccessRecords) TableName() string {
	return "public.invite_success_records"
}
