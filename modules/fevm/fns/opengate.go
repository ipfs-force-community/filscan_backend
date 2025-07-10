package fns

import (
	"fmt"
	"strings"
	
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
)

func NewOpengateToken() *OpengateToken {
	
	var err error
	t := &OpengateToken{
		Contracts: &Contracts{},
	}
	
	t.registrarABI, err = getABI("/opengate-fns/IRegistrar.json")
	if err != nil {
		panic(err)
	}
	t.registrarContract = "0xb8d7ca6a3253c418e52087693ca688d3257d70d1"
	
	t.registrarControllerABI, err = getABI("/opengate-fns/IRegistrarController.json")
	if err != nil {
		panic(err)
	}
	t.registrarControllerContract = "0xbA5cD7EF1414c33E3a250Fb89aD7C8F49844762d"
	
	t.publicResolverABI, err = getABI("/opengate-fns/PublicResolver.json")
	if err != nil {
		panic(err)
	}
	t.publicResolverContract = "0xE1b0C92be9C3855835dDbd0EF4c80DEe8fE41f60"
	
	t.fnsRegistryABI, err = getABI("/opengate-fns/FNSRegistry.json")
	if err != nil {
		panic(err)
	}
	t.fnsRegistryContract = "0xD4A059076d6C67c54113308344f94622728c4553"
	
	t.reverseRegistrarABI, err = getABI("/opengate-fns/ReverseRegistrar.json")
	if err != nil {
		panic(err)
	}
	t.reverseRegistrarContract = "0xC5C9FbA1B420F4Ad6D0662D08306551391C857Bb"
	
	return t
}

func NewOpengateTokenWithDomain(decoder fevm.FNSAPI, domain string) *OpengateToken {
	r := NewOpengateToken()
	r.decoder = decoder
	r.domain = domain
	return r
}

func NewOpengateTokenWithNode(decoder fevm.FNSAPI, node string) *OpengateToken {
	r := NewOpengateToken()
	r.decoder = decoder
	r.node = node
	return r
}

func NewOpengateTokenWithTokenID(decoder fevm.FNSAPI, tokenId string) *OpengateToken {
	r := NewOpengateToken()
	r.decoder = decoder
	r.tokenId = tokenId
	return r
}

var _ Token = (*OpengateToken)(nil)

type OpengateToken struct {
	domain   string
	tokenId  string
	node     string
	decoder  fevm.FNSAPI
	provider string
	*Contracts
}

func (o *OpengateToken) Decoder() fevm.FNSAPI {
	return o.decoder
}

func (o *OpengateToken) SetDecoder(decoder fevm.FNSAPI) {
	o.decoder = decoder
}

func (o *OpengateToken) Provider() string {
	return o.provider
}

func (o *OpengateToken) SetProvider(provider string) {
	o.provider = provider
}

func (o *OpengateToken) Domain() (domain string, err error) {
	if o.domain != "" {
		domain = o.domain
		return
	}
	
	if o.node != "" {
		domain, err = o.getDomainByNode(o.node)
		if err != nil {
			return
		}
		return
	}
	
	if o.tokenId != "" {
		var name string
		name, err = o.decoder.FNSName(fevm.Contract{
			ABI:     o.registrarABI,
			Address: o.registrarContract,
		}, o.tokenId)
		if err != nil {
			return
		}
		domain = fmt.Sprintf("%s.fil", name)
		o.domain = domain
		return
	}
	
	err = fmt.Errorf("cat't get domain")
	
	return
}

func (o *OpengateToken) getDomainByNode(node string) (domain string, err error) {
	
	defer func() {
		if err == nil && domain == "" {
			err = fmt.Errorf("opengate get domain by node error, node: %s", node)
		}
	}()
	
	controller, err := o.decoder.FNSOwner(fevm.Contract{
		ABI:     o.fnsRegistryABI,
		Address: o.fnsRegistryContract,
	}, node)
	if err != nil {
		return
	}
	
	// TODO 增加数据查询缓存
	
	domains, err := o.decoder.FNSBalanceOf(fevm.Contract{
		ABI:     o.registrarABI,
		Address: o.registrarContract,
	}, controller)
	if err != nil {
		return
	}
	
	for i := int64(0); i < domains; i++ {
		var tokenId string
		
		tokenId, err = o.decoder.FNSTokenOfOwnerByIndex(fevm.Contract{
			ABI:     o.registrarABI,
			Address: o.registrarContract,
		}, controller, uint64(i))
		if err != nil {
			return
		}
		
		var name string
		name, err = o.decoder.FNSName(fevm.Contract{
			ABI:     o.registrarABI,
			Address: o.registrarContract,
		}, tokenId)
		if err != nil {
			err = fmt.Errorf("get fns name error: %s", err)
			return
		}
		_domain := fmt.Sprintf("%s.fil", name)
		
		var _node string
		_node, err = o.decoder.FNSTokenNode(_domain)
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

func (o *OpengateToken) Node() (node string, err error) {
	if o.node != "" {
		node = o.node
		return
	}
	
	if o.domain != "" {
		node, err = o.decoder.FNSTokenNode(o.domain)
		if err != nil {
			return
		}
		o.node = node
		return
	}
	
	if o.tokenId != "" {
		var name string
		name, err = o.decoder.FNSName(fevm.Contract{
			ABI:     o.registrarABI,
			Address: o.registrarContract,
		}, o.tokenId)
		if err != nil {
			return
		}
		o.domain = fmt.Sprintf("%s.fil", name)
		
		node, err = o.decoder.FNSTokenNode(o.domain)
		if err != nil {
			return
		}
		o.node = node
		return
	}
	
	err = fmt.Errorf("cant't get node")
	
	return
}

func (o *OpengateToken) TokenId() (tokenId string, err error) {
	if o.tokenId != "" {
		tokenId = o.tokenId
		return
	}
	
	if o.domain != "" {
		tokenId, err = o.decoder.FNSTokenId(o.domain)
		if err != nil {
			return
		}
		o.tokenId = tokenId
		return
	}
	
	if o.node != "" {
		var domain string
		domain, err = o.getDomainByNode(o.node)
		if err != nil {
			return
		}
		o.domain = domain
		
		tokenId, err = o.decoder.FNSTokenId(o.domain)
		if err != nil {
			return
		}
		o.tokenId = tokenId
		return
	}
	
	err = fmt.Errorf("cant't get tokenId")
	
	return
}

func (o *OpengateToken) ExpiredAt() (expiredAt int64, err error) {
	
	domain, err := o.Domain()
	if err != nil {
		return
	}
	
	expiredAt, err = o.decoder.FNSNameExpires(fevm.Contract{
		ABI:     o.registrarControllerABI,
		Address: o.registrarControllerContract,
	}, strings.TrimRight(domain, ".fil"))
	if err != nil {
		return
	}
	
	return
}

func (o *OpengateToken) Registrant() (registrant string, err error) {
	
	tokenId, err := o.TokenId()
	if err != nil {
		return
	}
	
	registrant, err = o.decoder.FNSOwnerOf(fevm.Contract{
		ABI:     o.registrarABI,
		Address: o.registrarContract,
	}, tokenId)
	if err != nil {
		return
	}
	
	return
}

func (o *OpengateToken) Controller() (controller string, err error) {
	
	node, err := o.Node()
	if err != nil {
		return
	}
	
	controller, err = o.decoder.FNSOwner(fevm.Contract{
		ABI:     o.fnsRegistryABI,
		Address: o.fnsRegistryContract,
	}, node)
	if err != nil {
		return
	}
	
	return
}

func (o *OpengateToken) FilResolved() (filResolved string, err error) {
	
	node, err := o.Node()
	if err != nil {
		return
	}
	
	filResolved, err = o.decoder.FNSFilAddr(fevm.Contract{
		ABI:     o.publicResolverABI,
		Address: o.publicResolverContract,
	}, node)
	if err != nil {
		return
	}
	
	return
}

func (o *OpengateToken) ControllerTokens(controller string) (tokenIds []string, err error) {
	domains, err := o.decoder.FNSBalanceOf(fevm.Contract{
		ABI:     o.registrarABI,
		Address: o.registrarContract,
	}, controller)
	if err != nil {
		return
	}
	for i := int64(0); i < domains; i++ {
		var tokenId string
		tokenId, err = o.decoder.FNSTokenOfOwnerByIndex(fevm.Contract{
			ABI:     o.registrarABI,
			Address: o.registrarContract,
		}, controller, uint64(i))
		if err != nil {
			return
		}
		tokenIds = append(tokenIds, tokenId)
	}
	return
}
