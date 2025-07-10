package propo

import "time"

type User struct {
	Id          int64
	Name        *string
	Mail        string
	Password    string
	LoginAt     time.Time
	LastLoginAt time.Time
	CreatedAt   time.Time
	IsActivity  bool
}

func (User) TableName() string {
	return "pro.users"
}
