package decoder_test

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"testing"
	
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stretchr/testify/require"
	fevm "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/api"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/decoder"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/fevm/fns"
)

func TestFNSBalanceOf(t *testing.T) {
	//d := getAbiDecoder(t, getConf(t))
	
	conn, err := ethclient.Dial("https://api.node.glif.io/rpc/v1")
	if err != nil {
		fmt.Println("Dial err", err)
		return
	}
	fnsDecoder := decoder.NewFNS(conn)
	
	filfox := fns.NewOpengateTokenWithNode(fnsDecoder, "")
	isOpengate := false
	
	//filfox := fns.NewFilfoxTokenWithNode(fnsDecoder, "")
	//isOpengate := true
	
	addr := "0x8c3ecf71bba1356c6988ef0d71860f05c2fc4195"
	
	domains, err := fnsDecoder.FNSBalanceOf(fevm.Contract{
		ABI:     filfox.RegistrarABI(),
		Address: filfox.RegistrarContract(),
	}, addr)
	
	require.NoError(t, err)
	
	t.Log(domains)
	
	tokenId, err := fnsDecoder.FNSTokenOfOwnerByIndex(fevm.Contract{
		ABI:     filfox.RegistrarABI(),
		Address: filfox.RegistrarContract(),
	}, addr, uint64(0))
	require.NoError(t, err)
	t.Log("tokenId:", tokenId)
	
	masterNode, err := fnsDecoder.FNSNode(fevm.Contract{
		ABI:     filfox.ReverseRegistrarABI(),
		Address: filfox.ReverseRegistrarContract(),
	}, addr)
	require.NoError(t, err)
	t.Log("master node:", masterNode)
	
	var name string
	if !isOpengate {
		name, err = fnsDecoder.FNSGetNameByNode(fevm.Contract{
			ABI:     filfox.RegistrarABI(),
			Address: filfox.RegistrarContract(),
		}, masterNode)
		require.NoError(t, err)
		
		t.Log("master name:", name)
		
		name, err = fnsDecoder.FNSNameOf(fevm.Contract{
			ABI:     filfox.RegistrarABI(),
			Address: filfox.RegistrarContract(),
		}, tokenId)
		require.NoError(t, err)
		
		t.Log("name:", name)
		
	} else {
		
		name, err = fnsDecoder.FNSNodeName(fevm.Contract{
			ABI:     filfox.PublicResolverABI(),
			Address: filfox.PublicResolverContract(),
		}, masterNode)
		require.NoError(t, err)
		
		t.Log("master name:", name)
		
		name, err = fnsDecoder.FNSName(fevm.Contract{
			ABI:     filfox.RegistrarABI(),
			Address: filfox.RegistrarContract(),
		}, tokenId)
		require.NoError(t, err)
		
		t.Log("name:", name)
	}
	
	node, err := fnsDecoder.FNSTokenNode(fmt.Sprintf("%s.fil", name))
	require.NoError(t, err)
	
	t.Logf("domain node: %s", node)
	
	owner, err := fnsDecoder.FNSOwner(fevm.Contract{
		ABI:     filfox.FNSRegistryABI(),
		Address: filfox.FNSRegistryContract(),
	}, node)
	require.NoError(t, err)
	
	t.Log("controller:", owner)
	
	ownerOf, err := fnsDecoder.FNSOwnerOf(fevm.Contract{
		ABI:     filfox.RegistrarABI(),
		Address: filfox.RegistrarContract(),
	}, tokenId)
	require.NoError(t, err)
	
	t.Log("registrant:", ownerOf)
	
	expiredAt, err := fnsDecoder.FNSNameExpires(fevm.Contract{
		ABI:     filfox.RegistrarControllerABI(),
		Address: filfox.RegistrarControllerContract(),
	}, name)
	require.NoError(t, err)
	
	t.Log("expiredAt:", expiredAt)
	
	filResolved, err := fnsDecoder.FNSFilAddr(fevm.Contract{
		ABI:     filfox.PublicResolverABI(),
		Address: filfox.PublicResolverContract(),
	}, node)
	require.NoError(t, err)
	
	t.Log("filResolved:", filResolved)
}

func TestEmptyEthAddr(t *testing.T) {
	t.Log(common.HexToAddress("0000000000000000000000000000000000000000000000000000000000000000"))
}
