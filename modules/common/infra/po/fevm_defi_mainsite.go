package po

type ProductMainSite struct {
	Product string `gorm:"column:product"`
	Url     string `gorm:"column:url"`
}

func (ProductMainSite) TableName() string {
	return "fevm.defi_mainsite"
}
