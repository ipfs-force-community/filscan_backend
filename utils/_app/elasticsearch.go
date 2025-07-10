package _app

import (
	"fmt"
	"github.com/elastic/go-elasticsearch/v8"
)

func NewElasticsearchClient() *elasticsearch.Client {
	cfg := elasticsearch.Config{
		Addresses: []string{
			"http://192.168.1.118:9200",
		},
		Username: "elastic",
		Password: "123456",
	}
	es, err := elasticsearch.NewClient(cfg)
	if err != nil {
		panic(fmt.Errorf("new elasticsearch client error: %s", err))
	}
	return es
}
