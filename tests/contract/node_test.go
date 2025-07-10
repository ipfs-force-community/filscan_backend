package contract

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/math"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"github.com/wealdtech/go-ens/v3"
	"testing"
)

func TestNameHash(t *testing.T) {
	name, err := ens.NameHash("tianyou.fil")
	require.NoError(t, err)
	
	t.Log(common.Bytes2Hex(name[:]))
}

func TestTokenID(t *testing.T) {
	//client, err := ethclient.Dial("https://api.node.glif.io/rpc/v0")
	//require.NoError(t, err)
	//name, err := ens.DeriveTokenID(client, "tianyou.fil")
	//require.NoError(t, err)
	//
	//t.Log(name)
	
	t.Log(DeriveTokenID("tianyou.fil"))
	t.Log(DeriveTokenID("tuxingsun.fil"))
}

func DeriveTokenID(domain string) (string, error) {
	if domain == "" {
		return "", errors.New("empty domain")
	}
	//_, err := Resolve(backend, domain)
	//if err != nil {
	//	return "", err
	//}
	var err error
	domain, err = ens.NormaliseDomain(domain)
	if err != nil {
		return "", err
	}
	
	domain, err = ens.DomainPart(domain, 1)
	if err != nil {
		return "", err
	}
	labelHash, err := ens.LabelHash(domain)
	if err != nil {
		return "", err
	}
	hash := fmt.Sprintf("%#x", labelHash)
	tokenId, ok := math.ParseBig256(hash)
	if !ok {
		return "", err
	}
	return common.Bytes2Hex(tokenId.Bytes()), nil
	
}
