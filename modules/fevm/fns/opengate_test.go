package fns

import (
	"testing"
	
	"github.com/stretchr/testify/require"
)

func TestOpengateDomain(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewOpengateTokenWithDomain(d, "tianyou.fil")
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
	require.Equal(t, "0x8c3ecf71BBa1356c6988eF0D71860f05c2FC4195", controller)
	
	expiredAt, err := token.ExpiredAt()
	require.NoError(t, err)
	require.Equal(t, int64(1715163450), expiredAt)
	
	filAddr, err := token.FilResolved()
	require.NoError(t, err)
	
	t.Log(filAddr)
	
}

func TestOpengateDomain2(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewOpengateTokenWithDomain(d, "tuxingsun.fil")
	domain, err := token.Domain()
	require.NoError(t, err)
	require.Equal(t, "tuxingsun.fil", domain)
	
	filAddr, err := token.FilResolved()
	require.NoError(t, err)
	
	t.Log(filAddr)
}

func TestOpengateNode(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewOpengateTokenWithNode(d, "0x0a96f8958e3a6a60406417b0dbdedda26e04beb5a694126054b1b8933e697752")
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
	require.Equal(t, "0x8c3ecf71BBa1356c6988eF0D71860f05c2FC4195", controller)
	
	expiredAt, err := token.ExpiredAt()
	require.NoError(t, err)
	require.Equal(t, int64(1715163450), expiredAt)
}

func TestOpengateTokenId(t *testing.T) {
	d := getFNSDecoder(t)
	
	token := NewOpengateTokenWithTokenID(d, "0x4f37162eb623c25a7a16712bcc92e6eeb3e7ad92ba51bbbd5e2a5e619573961a")
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
	require.Equal(t, "0x8c3ecf71BBa1356c6988eF0D71860f05c2FC4195", controller)
	
	expiredAt, err := token.ExpiredAt()
	require.NoError(t, err)
	require.Equal(t, int64(1715163450), expiredAt)
}
