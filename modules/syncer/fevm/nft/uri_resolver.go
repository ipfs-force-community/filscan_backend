package nft

import (
	"context"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/gozelle/fastjson"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/po"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/utils/_dal"
	"gorm.io/gorm"
	"strings"
	"time"
)

type Repo interface {
	UpdateTokenURL(ctx context.Context, contract, tokenId, url string) (err error)
}

func NewUrlDal(db *gorm.DB) *UrlDal {
	return &UrlDal{BaseDal: _dal.NewBaseDal(db)}
}

var _ Repo = (*UrlDal)(nil)

type UrlDal struct {
	*_dal.BaseDal
}

func (u UrlDal) UpdateTokenURL(ctx context.Context, contract, tokenId, url string) (err error) {
	tx, err := u.DB(ctx)
	if err != nil {
		return
	}
	item := po.NFTToken{}
	err = tx.Table(item.TableName()).Where("contract = ? and token_id = ?", contract, tokenId).Update("token_url", url).Error
	if err != nil {
		return
	}
	return
}

type URLResolver interface {
	Resolve(item string, uri string) string
}

var resolvers = map[string]URLResolver{
	"0xf7ceaa5da7305b87361f9db6a300bd6d74c674d2": &FILPunk{},
	"0xa02cbf1dc75058cc6f2f5b8e7a9087425f5248e3": &Contract0xa02cbf1dc75058cc6f2f5b8e7a9087425f5248e3{},
	"0xbc3a4453dd52d3820eab1498c4673c694c5c6f09": &Contract0xbc3a4453dd52d3820eab1498c4673c694c5c6f09{resty: resty.New()},
}

func ResolveNFTURL(ctx context.Context, update bool, repo Repo, contract, tokenId, item, uri, url string) string {
	if url != "" {
		return url
	}
	r, ok := resolvers[contract]
	if !ok {
		return ""
	}
	url = r.Resolve(item, uri)
	if url != "" && update {
		_ = repo.UpdateTokenURL(ctx, contract, tokenId, url)
	}
	return url
}

var _ URLResolver = (*FILPunk)(nil)

type FILPunk struct {
}

func (F FILPunk) Resolve(item, uri string) string {
	return fmt.Sprintf("https://ipfs.io/ipfs/bafybeih5tp5mcmdya2reg3pruembinv4kjtfrryhdirzkwe2ready2752u/%s.png", item)
}

var _ URLResolver = (*Contract0xa02cbf1dc75058cc6f2f5b8e7a9087425f5248e3)(nil)

// Ricardo Goulart
type Contract0xa02cbf1dc75058cc6f2f5b8e7a9087425f5248e3 struct {
}

func (Contract0xa02cbf1dc75058cc6f2f5b8e7a9087425f5248e3) Resolve(item, uri string) string {
	return fmt.Sprintf("https://ipfs.io/ipfs/QmWW6mhZtGQ8HA7cA7bxe8egi4G7CYQeQCEr726pGPxw5R/%s.png", item)
}

var _ URLResolver = (*Contract0xbc3a4453dd52d3820eab1498c4673c694c5c6f09)(nil)

// FileBunnies
type Contract0xbc3a4453dd52d3820eab1498c4673c694c5c6f09 struct {
	resty *resty.Client
}

func (c Contract0xbc3a4453dd52d3820eab1498c4673c694c5c6f09) Resolve(item string, uri string) string {
	if uri == "" {
		return ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()
	resp, err := c.resty.R().SetContext(ctx).Get(fmt.Sprintf("https://gateway.lighthouse.storage/ipfs/%s", strings.TrimPrefix(uri, "ipfs://")))
	if err != nil {
		return ""
	}
	v, err := fastjson.Parse(resp.String())
	if err != nil {
		return ""
	}
	return fmt.Sprintf("https://gateway.lighthouse.storage/ipfs/%s", strings.TrimPrefix(string(v.GetStringBytes("image")), "ipfs://"))
}
