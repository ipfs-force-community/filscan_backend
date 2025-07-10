package po

type FEvmMethods struct {
	Id            string `gorm:"column:id"`
	TextSignature string `gorm:"column:text_signature"`
	HexSignature  string `gorm:"column:hex_signature"`
	Decode        string `gorm:"column:decode"`
}

func (FEvmMethods) TableName() string {
	return "fevm.methods"
}
