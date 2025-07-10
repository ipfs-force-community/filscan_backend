package evm_signature

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"reflect"
	"strings"
)

func NewAPI4Byte(host string, client *resty.Client) *API4Byte {
	return &API4Byte{host: host, client: client}
}

type API4Byte struct {
	host   string
	client *resty.Client
}

func (a API4Byte) exec(ctx context.Context, path string, body map[string]interface{}) (*resty.Response, error) {
	r := a.client.R()
	r.SetContext(ctx)
	r.SetBody(body)

	return r.Get(fmt.Sprintf("%s/%s", strings.TrimRight(a.host, "/"), strings.TrimLeft(path, "/")))
}

func (a API4Byte) bindResult(resp *resty.Response, result interface{}) (err error) {
	defer func() {
		if err != nil {
			result = nil
		}
	}()
	if reflect.TypeOf(result).Kind() != reflect.Ptr {
		err = fmt.Errorf("only accept pointer")
		return
	}
	if resp.Request == nil {
		resp.Request = &resty.Request{Method: "unknown", URL: "unknown"}
	}
	err = json.Unmarshal([]byte(resp.String()), result)
	if err != nil {
		err = fmt.Errorf("unmarshal error:%s", err)
		return
	}
	return
}

func (a API4Byte) EventSignature(ctx context.Context, page int) (result *EventSignatureList, err error) {
	path := fmt.Sprintf("/api/v1/event-signatures/?page=%d", page)

	resp, err := a.exec(ctx, path, map[string]interface{}{})
	if err != nil {
		return
	}
	err = a.bindResult(resp, &result)
	if err != nil {
		return
	}
	return
}
