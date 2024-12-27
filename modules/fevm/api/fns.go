package fevm

type FNSAPI interface {
	FNSTokenNode(domain string) (node string, err error)
	FNSTokenId(domain string) (tokenId string, err error)
	FNSBalanceOf(contract Contract, address string) (domains int64, err error)
	FNSNode(contract Contract, addr string) (node string, err error)
	//FNSAddr(contract Contract, node string) (addr string, err error)
	FNSTokenOfOwnerByIndex(contract Contract, address string, index uint64) (tokenID string, err error)
	FNSNodeName(contract Contract, node string) (name string, err error)
	FNSGetNameByNode(contract Contract, node string) (name string, err error)
	FNSName(contract Contract, tokenID string) (name string, err error)
	FNSNameOf(contract Contract, tokenID string) (name string, err error)
	FNSAvailable(contract Contract) (err error)
	FNSOwnerOf(contract Contract, tokenID string) (address string, err error)
	FNSOwner(contract Contract, node string) (owner string, err error)
	FNSNameExpires(contract Contract, name string) (expiredAt int64, err error)
	FNSFilAddr(contract Contract, node string) (str string, err error)
}

type Contract struct {
	ABI     []byte
	Address string
}
