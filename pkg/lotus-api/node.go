package lotus_api

import (
	"context"
	"encoding/base64"
	"github.com/filecoin-project/go-jsonrpc"
	"github.com/filecoin-project/lotus/api"
	"net/http"
	"time"
)

const GLIF_RPC = "https://api.node.glif.io/rpc/v0"
const NOID = "noid"

type Closer struct {
	closer jsonrpc.ClientCloser
}

type Config struct {
	name     string
	rpc      string
	user     string
	password string
	driver   string
	delay    time.Duration
}

func WithNoID() func(*Config) {
	return func(conf *Config) {
		conf.driver = NOID
	}
}

func WithDelay(delay time.Duration) func(*Config) {
	return func(conf *Config) {
		conf.delay = delay
	}
}

func WithAuth(user, password string) func(*Config) {
	return func(conf *Config) {
		conf.user = user
		conf.password = password
	}
}

func (p *Closer) Close() {
	defer func() {
		_ = recover()
	}()
	p.closer()
}

func basicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

func NewBasicAuthLotusApi(name, rpc string, c ...func(conf *Config)) (node *Node, err error) {
	conf := &Config{
		name: name,
		rpc:  rpc,
	}
	for _, v := range c {
		v(conf)
	}
	_api := new(api.FullNodeStruct)
	closer := new(Closer)
	outs := []interface{}{&_api.Internal, &_api.CommonStruct.Internal}
	var headers http.Header
	if conf.user != "" && conf.password != "" {
		headers = http.Header{"Authorization": []string{"Basic " + basicAuth(conf.user, conf.password)}}
	} else if conf.password != "" {
		headers = http.Header{"Authorization": []string{"Bearer " + conf.password}}
	}
	closer.closer, err = jsonrpc.NewMergeClient(context.Background(), conf.rpc, "Filecoin", outs, headers)
	if err != nil {
		return
	}
	node = new(Node)
	node.Name = name
	node.FullNodeStruct = _api
	node.closer = closer
	node.Delay = conf.delay
	return
}

type Node struct {
	Name  string
	Delay time.Duration
	*api.FullNodeStruct
	closer *Closer
}

func (p *Node) Close() {
	p.closer.Close()
}
