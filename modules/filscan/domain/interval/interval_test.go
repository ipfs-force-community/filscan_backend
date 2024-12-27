package interval

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestResolveType(t *testing.T) {
	t.Log(ResolveType("7d"))
}

func Test24HPoints(t *testing.T) {
	i, err := ResolveInterval("3d", 3204720)
	require.NoError(t, err)
	
	//require.Equal(t, 25, len(i.Points()))
	t.Log(len(i.Points()))
	for _, v := range i.Points() {
		t.Log(v.Int64(), v.Format())
	}
}
 