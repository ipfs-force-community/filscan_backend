package po

type NFTContract struct {
	Contract   string
	Collection string
	Logo       string
	Owners     int64
	Transfers  int64
	Mints      int64
}

func (NFTContract) TableName() string {
	return "fevm.nft_contracts"
}
