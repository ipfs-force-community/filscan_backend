package po

type FilPrice struct {
	Id            int     `gorm:"column:id"`
	Price         float64 `gorm:"column:price"`
	PercentChange float64 `gorm:"column:percent_change_24h"`
	Timestamp     int64   `gorm:"column:timestamp"`
}

func (FilPrice) TableName() string {
	return "public.fil_price"
}
