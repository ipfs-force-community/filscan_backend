package po

type Banner struct {
	Id       int
	Category string
	Url      string
	Orders   int
	Language string
	Link     string
}

func (Banner) TableName() string {
	return "public.banner"
}
