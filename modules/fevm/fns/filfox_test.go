package fns

import (
	"testing"
	
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/decoder"
)

func getFNSDecoder(t *testing.T) fevm.FNSAPI {
	conn, err := ethclient.Dial("https://api.node.glif.io/rpc/v1")
	//conn, err := ethclient.Dial("https://filfox.info/rpc/v1")
	//conn, err := ethclient.Dial("http://127.0.0.1:10000/proxy/glif")
	if err != nil {
		require.NoError(t, err)
	}
	return decoder.NewFNS(conn)
}

func TestFilfoxDomain(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewFilfoxTokenWithDomain(d, "tianyou.fil")
	domain, err := token.Domain()
	require.NoError(t, err)
	require.Equal(t, "tianyou.fil", domain)
	
	node, err := token.Node()
	require.NoError(t, err)
	require.Equal(t, "0x0a96f8958e3a6a60406417b0dbdedda26e04beb5a694126054b1b8933e697752", node)
	
	tokenId, err := token.TokenId()
	require.NoError(t, err)
	require.Equal(t, "0x4f37162eb623c25a7a16712bcc92e6eeb3e7ad92ba51bbbd5e2a5e619573961a", tokenId)
	
	registrant, err := token.Registrant()
	require.NoError(t, err)
	require.Equal(t, "0x8c3ecf71BBa1356c6988eF0D71860f05c2FC4195", registrant)
	
	controller, err := token.Controller()
	require.NoError(t, err)
	require.Equal(t, "0x3F46A956e1024a964EDFB864aeDd1179B2024892", controller)
	
	expiredAt, err := token.ExpiredAt()
	require.NoError(t, err)
	require.Equal(t, int64(1718678580), expiredAt)
	
	filAddr, err := token.FilResolved()
	require.NoError(t, err)
	require.Equal(t, "sss", filAddr)
}

func TestFilfoxNode(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewFilfoxTokenWithNode(d, "0x0a96f8958e3a6a60406417b0dbdedda26e04beb5a694126054b1b8933e697752")
	domain, err := token.Domain()
	require.NoError(t, err)
	require.Equal(t, "tianyou.fil", domain)
	
	node, err := token.Node()
	require.NoError(t, err)
	require.Equal(t, "0x0a96f8958e3a6a60406417b0dbdedda26e04beb5a694126054b1b8933e697752", node)
	
	tokenId, err := token.TokenId()
	require.NoError(t, err)
	require.Equal(t, "0x4f37162eb623c25a7a16712bcc92e6eeb3e7ad92ba51bbbd5e2a5e619573961a", tokenId)
	
	registrant, err := token.Registrant()
	require.NoError(t, err)
	require.Equal(t, "0x8c3ecf71BBa1356c6988eF0D71860f05c2FC4195", registrant)
	
	controller, err := token.Controller()
	require.NoError(t, err)
	require.Equal(t, "0x3F46A956e1024a964EDFB864aeDd1179B2024892", controller)
	
	expiredAt, err := token.ExpiredAt()
	require.NoError(t, err)
	require.Equal(t, int64(1718678580), expiredAt)
	
	filAddr, err := token.FilResolved()
	require.NoError(t, err)
	require.Equal(t, "sss", filAddr)
}

func TestFilfoxNode2(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewFilfoxTokenWithNode(d, "0x2a715e7809da761cf3802a90983b510abb19edee964b5deed94878b8fe85ed6d")
	domain, err := token.Domain()
	require.NoError(t, err)
	//require.Equal(t, "tianyou.fil", domain)
	t.Log(domain)
	
	tokenId, err := token.TokenId()
	require.NoError(t, err)
	t.Log(tokenId)
	//require.Equal(t, "0x4f37162eb623c25a7a16712bcc92e6eeb3e7ad92ba51bbbd5e2a5e619573961a", tokenId)
	//
	
	//node, err := d.FNSTokenNode(domain)
	node, err := token.Node()
	require.NoError(t, err)
	t.Log(node)
	//require.Equal(t, "0x0a96f8958e3a6a60406417b0dbdedda26e04beb5a694126054b1b8933e697752", node)
	//
	
	controller, err := token.Controller()
	require.NoError(t, err)
	t.Log(controller)
	//require.Equal(t, "0x3F46A956e1024a964EDFB864aeDd1179B2024892", controller)
	//
	
	registrant, err := token.Registrant()
	require.NoError(t, err)
	t.Log(registrant)
	//require.Equal(t, "0x8c3ecf71BBa1356c6988eF0D71860f05c2FC4195", registrant)
	//
	
	expiredAt, err := token.ExpiredAt()
	require.NoError(t, err)
	t.Log(expiredAt)
	//require.Equal(t, int64(1718678580), expiredAt)
	//
	filAddr, err := token.FilResolved()
	require.NoError(t, err)
	t.Log(filAddr)
	//require.Equal(t, "sss", filAddr)
	
}

func TestFilfoxTokenId(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewFilfoxTokenWithTokenID(d, "0x4f37162eb623c25a7a16712bcc92e6eeb3e7ad92ba51bbbd5e2a5e619573961a")
	domain, err := token.Domain()
	require.NoError(t, err)
	require.Equal(t, "tianyou.fil", domain)
	
	node, err := token.Node()
	require.NoError(t, err)
	require.Equal(t, "0x0a96f8958e3a6a60406417b0dbdedda26e04beb5a694126054b1b8933e697752", node)
	
	tokenId, err := token.TokenId()
	require.NoError(t, err)
	require.Equal(t, "0x4f37162eb623c25a7a16712bcc92e6eeb3e7ad92ba51bbbd5e2a5e619573961a", tokenId)
	
	registrant, err := token.Registrant()
	require.NoError(t, err)
	require.Equal(t, "0x8c3ecf71BBa1356c6988eF0D71860f05c2FC4195", registrant)
	
	controller, err := token.Controller()
	require.NoError(t, err)
	require.Equal(t, "0x3F46A956e1024a964EDFB864aeDd1179B2024892", controller)
	
	expiredAt, err := token.ExpiredAt()
	require.NoError(t, err)
	require.Equal(t, int64(1718678580), expiredAt)
	
	filAddr, err := token.FilResolved()
	require.NoError(t, err)
	require.Equal(t, "sss", filAddr)
}
