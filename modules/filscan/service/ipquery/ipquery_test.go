package ipquery

import (
	"encoding/base64"
	"encoding/json"
	"github.com/gozelle/spew"
	"github.com/multiformats/go-multiaddr"
	"github.com/stretchr/testify/require"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
	"testing"
)

func TestDecodeMultiaddrs(t *testing.T) {
	
	const minerInfo = `{
            "ID": "bafy2bzacednt224uu6pqze5323ppdxkp75d3ldyz5wqna2q3hz3ymo4sij27c",
            "Epoch": 2628720,
            "Miner": "065103",
            "Multiaddrs": [
                {
                    "Subtype": 0,
                    "Data": "BC/yTmQGPow="
                },
                {
                    "Subtype": 0,
                    "Data": "BC/yTv4GPow="
                },
                {
                    "Subtype": 0,
                    "Data": "BC/yRwMGPow="
                }
            ]
        }`
	
	info := new(londobell.MinerInfo)
	err := json.Unmarshal([]byte(minerInfo), info)
	if err != nil {
		return
	}
	require.NoError(t, err)
	
	var multis []string
	for _, v := range info.Multiaddrs {
		var maddr multiaddr.Multiaddr
		var d []byte
		d, err = base64.StdEncoding.DecodeString(v.Data)
		require.NoError(t, err)
		
		maddr, err = multiaddr.NewMultiaddrBytes(d)
		if err != nil {
			require.NoError(t, err)
			return
		}
		multis = append(multis, maddr.String())
	}
	
	spew.Json(multis)
}

func TestQueryIp(t *testing.T) {
	iq := NewIpQuery()
	r, err := iq.Query("8.8.8.8")
	require.NoError(t, err)
	spew.Json(r)
}
