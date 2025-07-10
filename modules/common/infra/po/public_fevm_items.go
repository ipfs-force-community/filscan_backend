package po

type FEvmItem struct {
	Id       int
	Twitter  string
	MainSite string `gorm:"column:main_site"`
	Name     string
	Logo     string
}

func (FEvmItem) TableName() string {
	return "public.fevm_item"
}

type FEvmItemCategory struct {
	Id       int
	ItemId   int `gorm:"column:item_id"`
	Category string
	Orders   int
}

func (FEvmItemCategory) TableName() string {
	return "public.fevm_item_category"
}

type FEvmHotItem struct {
	Id       int
	ItemId   int `gorm:"column:item_id"`
	Category string
	Orders   int
}

func (FEvmHotItem) TableName() string {
	return "public.hot_fevm_items"
}
