package _app

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNewElasticsearchClient(t *testing.T) {
	c := NewElasticsearchClient()
	
	r0, err := c.Info()
	t.Log(err)
	require.NoError(t, err)
	t.Log(r0.String())
	
	r, err := c.Ping()
	require.NoError(t, err)
	t.Log(r.String())
	
	body := &bytes.Buffer{}
	err = json.NewEncoder(body).Encode(map[string]string{
		"name": "apple",
	})
	require.NoError(t, err)
	
	resp, err := c.Create("fruits", "1", body)
	require.NoError(t, err)
	require.False(t, resp.IsError())
}
