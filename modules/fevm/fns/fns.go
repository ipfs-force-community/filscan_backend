package fns

type Token interface {
	Domain() (domain string, err error)
	Node() (node string, err error)
	TokenId() (tokenId string, err error)
	Registrant() (registrant string, err error)
	Controller() (controller string, err error)
	ExpiredAt() (expiredAt int64, err error)
	FilResolved() (filResolved string, err error)
	SetProvider(provider string)
	ControllerTokens(controller string) (tokenIds []string, err error)
}

type Contract interface {
	RegistrarContract() string
	RegistrarControllerContract() string
	PublicResolverContract() string
	FNSRegistryContract() string
	ReverseRegistrarContract() string
	RegistrarABI() []byte
	RegistrarControllerABI() []byte
	PublicResolverABI() []byte
	FNSRegistryABI() []byte
	ReverseRegistrarABI() []byte
}
