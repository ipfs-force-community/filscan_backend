package po

import "time"

type Events struct {
	ID       int64     `gorm:"column:id"`
	ImageUrl string    `gorm:"column:image_url"`
	JumpUrl  string    `gorm:"column:jump_url"`
	StartAt  time.Time `gorm:"column:start_at"`
	EndAt    time.Time `gorm:"column:end_at"`
	Name     string    `gorm:"column:name"`
}

func (Events) TableName() string {
	return "public.events"
}
