package selector

import (
	"context"
	"fmt"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/types"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	lotus_api "gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/lotus-api"
	"time"
)

// SelectNode 检查高度，获取可用节点，并检查是否空高度
// 当 empty = true 时，若 epoch = 100, 则返回 tipset.Height() 取 < 100 链有的最大高度，即上一个高度
func SelectNode(epoch chain.Epoch, nodes ...*lotus_api.Node) (node *lotus_api.Node, tipset *types.TipSet, empty bool, err error) {
	
	// 默认超时时间: 5秒钟
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer func() {
		cancel()
	}()
	
	// 寻找高度最高的节点
	head := &types.TipSet{}
	var index int
	for i, v := range nodes {
		var t *types.TipSet
		t, err = v.ChainHead(ctx)
		if err != nil {
			continue
		}
		if t.Height() > head.Height() {
			head = t
			index = i
		}
	}
	
	if head.Height() == abi.ChainEpoch(epoch) {
		node = nodes[index]
		tipset = head
		return
	} else if head.Height() > abi.ChainEpoch(epoch) {
		node = nodes[index]
		var t *types.TipSet
		t, err = node.ChainGetTipSetByHeight(ctx, abi.ChainEpoch(epoch), types.EmptyTSK)
		if err != nil {
			return
		}
		
		tipset = t
		
		if t.Height() == abi.ChainEpoch(epoch) {
			return
		} else if t.Height() < abi.ChainEpoch(epoch) {
			empty = true
			return
		}
	}
	err = fmt.Errorf("高度: %d 无节点可用", epoch)
	return
}
