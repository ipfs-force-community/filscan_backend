package filfox_fns

import (
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/bundle"
	"io"
)

func getABI(path string) ([]byte, error) {
	f, err := bundle.Templates.Open(path)
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = f.Close()
	}()
	
	c, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	
	return c, nil
}

func NewFilfoxFNS() *OpengateFNS {
	
	var err error
	t := &OpengateFNS{}
	
	t.RegistrarABI, err = getABI("/filfox-fns/Registrar.json")
	if err != nil {
		panic(err)
	}
	t.RegistrarContract = "0x45d9d6408d5159a379924cf423cb7e15C00fA81f"
	
	t.RegistrarControllerABI, err = getABI("/filfox-fns/RegistrarController.json")
	if err != nil {
		panic(err)
	}
	t.RegistrarControllerContract = "0xDA3c407a23Ef96930f1A07903fB8360D8926991E"
	
	t.PublicResolverABI, err = getABI("/filfox-fns/PublicResolver.json")
	if err != nil {
		panic(err)
	}
	t.PublicResolverContract = "0xed9bd04b1BB87Abe2EfF583A977514940c95699c"
	
	t.FNSRegistryABI, err = getABI("/filfox-fns/FNSRegistry.json")
	if err != nil {
		panic(err)
	}
	t.FNSRegistryContract = "0x916915d0d41EaA8AAEd70b2A5Fb006FFc213961b"
	
	t.ReverseRegistrarABI, err = getABI("/filfox-fns/ReverseRegistrar.json")
	if err != nil {
		panic(err)
	}
	t.ReverseRegistrarContract = "0xc49833d827b01e1465c65221A59885Fb71614a26"
	
	return t
}

type OpengateFNS struct {
	RegistrarContract           string
	RegistrarControllerContract string
	PublicResolverContract      string
	FNSRegistryContract         string
	ReverseRegistrarContract    string
	RegistrarABI                []byte
	RegistrarControllerABI      []byte
	PublicResolverABI           []byte
	FNSRegistryABI              []byte
	ReverseRegistrarABI         []byte
}
