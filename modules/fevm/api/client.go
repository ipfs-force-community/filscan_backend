package fevm

import (
	"github.com/gozelle/jsonrpc"
	"github.com/gozelle/mix/client"
)

type AbiDecoderClient struct {
	closer jsonrpc.ClientCloser
	ABIDecoderAPIStruct
}

func (c AbiDecoderClient) Close() {
	if c.closer != nil {
		c.closer()
	}
}

func NewAbiDecoderClient(addr string, opts ...client.Option) (c *AbiDecoderClient, err error) {
	c = &AbiDecoderClient{}
	opts = append(opts, client.WithOut(
		&c.ABIDecoderAPIStruct.Internal,
		&c.ContractAPIStruct.Internal,
		&c.FNSAPIStruct.Internal,
	))
	c.closer, err = client.NewClient(addr, "abi", opts...)
	if err != nil {
		return
	}
	return
}
