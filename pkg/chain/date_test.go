package chain

import (
	"github.com/golang-module/carbon/v2"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewDate(t *testing.T) {
	
	d, err := NewDate(carbon.Shanghai, "2023-08-14")
	require.NoError(t, err)
	
	t.Log(d.Epoch())
	
	d, err = NewDate(carbon.NewYork, "2023-08-14")
	require.NoError(t, err)
	
	t.Log(d.Epoch())
}
