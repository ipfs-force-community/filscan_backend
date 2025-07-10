package fns

import (
	"fmt"
	
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
)

func NewFilfoxToken() *FilfoxFns {
	
	var err error
	t := &FilfoxFns{
		OpengateToken: &OpengateToken{
			Contracts: &Contracts{},
		},
	}
	
	t.registrarABI, err = getABI("/filfox-fns/Registrar.json")
	if err != nil {
		panic(err)
	}
	t.registrarContract = "0x45d9d6408d5159a379924cf423cb7e15C00fA81f"
	
	t.registrarControllerABI, err = getABI("/filfox-fns/RegistrarController.json")
	if err != nil {
		panic(err)
	}
	t.registrarControllerContract = "0xDA3c407a23Ef96930f1A07903fB8360D8926991E"
	
	t.publicResolverABI, err = getABI("/filfox-fns/PublicResolver.json")
	if err != nil {
		panic(err)
	}
	t.publicResolverContract = "0xed9bd04b1BB87Abe2EfF583A977514940c95699c"
	
	t.fnsRegistryABI, err = getABI("/filfox-fns/FNSRegistry.json")
	if err != nil {
		panic(err)
	}
	t.fnsRegistryContract = "0x916915d0d41EaA8AAEd70b2A5Fb006FFc213961b"
	
	t.reverseRegistrarABI, err = getABI("/filfox-fns/ReverseRegistrar.json")
	if err != nil {
		panic(err)
	}
	t.reverseRegistrarContract = "0xc49833d827b01e1465c65221A59885Fb71614a26"
	
	return t
}

type Contracts struct {
	registrarContract           string
	registrarControllerContract string
	publicResolverContract      string
	fnsRegistryContract         string
	reverseRegistrarContract    string
	registrarABI                []byte
	registrarControllerABI      []byte
	publicResolverABI           []byte
	fnsRegistryABI              []byte
	reverseRegistrarABI         []byte
}

func (c Contracts) RegistrarContract() string {
	return c.registrarContract
}

func (c Contracts) RegistrarControllerContract() string {
	return c.registrarControllerContract
}

func (c Contracts) PublicResolverContract() string {
	return c.publicResolverContract
}

func (c Contracts) FNSRegistryContract() string {
	return c.fnsRegistryContract
}

func (c Contracts) ReverseRegistrarContract() string {
	return c.reverseRegistrarContract
}

func (c Contracts) RegistrarABI() []byte {
	return c.registrarABI
}

func (c Contracts) RegistrarControllerABI() []byte {
	return c.registrarControllerABI
}

func (c Contracts) PublicResolverABI() []byte {
	return c.publicResolverABI
}

func (c Contracts) FNSRegistryABI() []byte {
	return c.fnsRegistryABI
}

func (c Contracts) ReverseRegistrarABI() []byte {
	return c.reverseRegistrarABI
}

func NewFilfoxTokenWithDomain(decoder fevm.FNSAPI, domain string) *FilfoxFns {
	r := NewFilfoxToken()
	r.decoder = decoder
	r.domain = domain
	return r
}

func NewFilfoxTokenWithNode(decoder fevm.FNSAPI, node string) *FilfoxFns {
	r := NewFilfoxToken()
	r.decoder = decoder
	r.node = node
	return r
}

func NewFilfoxTokenWithTokenID(decoder fevm.FNSAPI, tokenId string) *FilfoxFns {
	r := NewFilfoxToken()
	r.decoder = decoder
	r.tokenId = tokenId
	return r
}

var _ Token = (*FilfoxFns)(nil)

type FilfoxFns struct {
	*OpengateToken
}

func (f FilfoxFns) Domain() (domain string, err error) {
	if f.domain != "" {
		domain = f.domain
		return
	}
	
	if f.node != "" {
		domain, err = f.getDomainByNode(f.node)
		if err != nil {
			return
		}
		return
	}
	
	if f.tokenId != "" {
		var name string
		name, err = f.decoder.FNSNameOf(fevm.Contract{
			ABI:     f.registrarABI,
			Address: f.registrarContract,
		}, f.tokenId)
		if err != nil {
			return
		}
		domain = fmt.Sprintf("%s.fil", name)
		f.domain = domain
		return
	}
	
	err = fmt.Errorf("cat't get domain")
	
	return
}

func (f FilfoxFns) getDomainByNode(node string) (domain string, err error) {
	
	defer func() {
		if err == nil && domain == "" {
			err = fmt.Errorf("filfox get domain by node error, node: %s", node)
		}
	}()
	
	controller, err := f.decoder.FNSOwner(fevm.Contract{
		ABI:     f.fnsRegistryABI,
		Address: f.fnsRegistryContract,
	}, node)
	if err != nil {
		return
	}
	
	// TODO 增加数据查询缓存
	
	domains, err := f.decoder.FNSBalanceOf(fevm.Contract{
		ABI:     f.registrarABI,
		Address: f.registrarContract,
	}, controller)
	if err != nil {
		return
	}
	
	for i := int64(0); i < domains; i++ {
		var tokenId string
		
		tokenId, err = f.decoder.FNSTokenOfOwnerByIndex(fevm.Contract{
			ABI:     f.registrarABI,
			Address: f.registrarContract,
		}, controller, uint64(i))
		if err != nil {
			return
		}
		
		var name string
		name, err = f.decoder.FNSNameOf(fevm.Contract{
			ABI:     f.registrarABI,
			Address: f.registrarContract,
		}, tokenId)
		if err != nil {
			err = fmt.Errorf("get fns name error: %s", err)
			return
		}
		_domain := fmt.Sprintf("%s.fil", name)
		
		var _node string
		_node, err = f.decoder.FNSTokenNode(_domain)
		if err != nil {
			return
		}
		
		if _node == node {
			domain = _domain
			return
		}
	}
	
	return
}

func (f FilfoxFns) Node() (node string, err error) {
	if f.node != "" {
		node = f.node
		return
	}
	
	if f.domain != "" {
		node, err = f.decoder.FNSTokenNode(f.domain)
		if err != nil {
			return
		}
		f.node = node
		return
	}
	
	if f.tokenId != "" {
		var name string
		name, err = f.decoder.FNSNameOf(fevm.Contract{
			ABI:     f.registrarABI,
			Address: f.registrarContract,
		}, f.tokenId)
		if err != nil {
			return
		}
		f.domain = fmt.Sprintf("%s.fil", name)
		
		node, err = f.decoder.FNSTokenNode(f.domain)
		if err != nil {
			return
		}
		f.node = node
		return
	}
	
	err = fmt.Errorf("cant't get node")
	
	return
}

func (f FilfoxFns) TokenId() (tokenId string, err error) {
	if f.tokenId != "" {
		tokenId = f.tokenId
		return
	}
	
	if f.domain != "" {
		tokenId, err = f.decoder.FNSTokenId(f.domain)
		if err != nil {
			return
		}
		f.tokenId = tokenId
		return
	}
	
	if f.node != "" {
		var domain string
		domain, err = f.decoder.FNSGetNameByNode(fevm.Contract{
			ABI:     f.registrarABI,
			Address: f.registrarContract,
		}, f.node)
		if err != nil {
			return
		}
		f.domain = domain
		
		tokenId, err = f.decoder.FNSTokenId(f.domain)
		if err != nil {
			return
		}
		f.tokenId = tokenId
		return
	}
	
	err = fmt.Errorf("cant't get tokenId")
	
	return
}
