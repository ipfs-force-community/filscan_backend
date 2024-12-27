package po

type NFTToken struct {
	TokenId   string  // NFT 的 Token ID, big.Int.String()
	Contract  string  // 合约的 0x 地址
	Name      string  // NFT 名称
	Symbol    string  // NFT Symbol
	TokenUri  string  // NFT Token URI
	TokenUrl  *string // NFT Token URI
	Owner     string
	InitEpoch int64
	Item      string
}

func (NFTToken) TableName() string {
	return "fevm.nft_tokens"
}
