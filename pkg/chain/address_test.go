package chain

import (
	"github.com/filecoin-project/go-address"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
)

func TestIsPureActorID(t *testing.T) {
	ok := IsPureActorID("3v2do53disemyutp6nt7xesprhuvxvig7vrxgkfzdchlly2g6jcgzzz45xyn5ojjlcs7vnv2pxqdhggpjvnva")
	require.Equal(t, false, ok)
	ok = IsPureActorID("05")
	require.Equal(t, true, ok)
	
	t.Log(filepath.Join("github.com/mix/mix", "../"))
	t.Log(filepath.Join("github.com", "../"))
}

func TestSmartAddress(t *testing.T) {
	sa := SmartAddress("02438")
	require.NoError(t, sa.Valid())
	require.Equal(t, "f02438", sa.Address())
	require.Equal(t, "02438", sa.CrudeAddress())
	
	sa2 := SmartAddress("f3v2do53disemyutp6nt7xesprhuvxvig7vrxgkfzdchlly2g6jcgzzz45xyn5ojjlcs7vnv2pxqdhggpjvnva")
	require.NoError(t, sa.Valid())
	require.Equal(t, "f3v2do53disemyutp6nt7xesprhuvxvig7vrxgkfzdchlly2g6jcgzzz45xyn5ojjlcs7vnv2pxqdhggpjvnva", sa2.Address())
	require.Equal(t, "3v2do53disemyutp6nt7xesprhuvxvig7vrxgkfzdchlly2g6jcgzzz45xyn5ojjlcs7vnv2pxqdhggpjvnva", sa2.CrudeAddress())
}

func TestIsID(t *testing.T) {
	ok := SmartAddress("02438").IsID()
	
	require.True(t, ok)
	
	ok = SmartAddress("f3v2do53disemyutp6nt7xesprhuvxvig7vrxgkfzdchlly2g6jcgzzz45xyn5ojjlcs7vnv2pxqdhggpjvnva").IsID()
	require.True(t, !ok)
}

func TestEmptyAddress(t *testing.T) {
	a := address.Address{}
	t.Log(a.String())
	
	b, err := address.NewFromString("<empty>")
	require.NoError(t, err)
	t.Log(b.String())
}

func TestTestNet(t *testing.T) {
	RegisterNet(true)
	t.Log(address.CurrentNetwork)
	t.Log(SmartAddress("bafy2bzaceaflyqz6ovwqwcka74yehbr6jeceeje72sg67wwp46sj6tzj2cacw").Valid())
}

func TestTrimHexAddress(t *testing.T) {
	t.Log(TrimHexAddress("0x0000000000000000000000004368ff3d6203fad0947b052d1f8abe0526886260"))
}
