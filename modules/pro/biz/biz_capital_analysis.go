package probiz

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/redis"

	prodal "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/infra/dal"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/repository"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/common/infra/convertor"
	"gorm.io/gorm"

	"github.com/shopspring/decimal"

	logging "github.com/gozelle/logger"
	"github.com/gozelle/mix"
	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/robfig/cron"
	pro "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/pro/api"
	capital_task "gitlab.forceup.in/fil-data-factory/filscan-backend/modules/syncer/capital"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/londobell"
)

var _ pro.CapitalAnalysisAPI = (*CapitalAnalysisBiz)(nil)
var cache *expirable.LRU[string, []string]

const LAYERNUMS = 3 //todo 先定义为3层，后续扩展
const WIDTHNUMS = 3

var logger = logging.NewLogger("capital")
var once sync.Once

type CapitalAnalysisBiz struct {
	Agg         londobell.Agg
	Adapter     londobell.Adapter
	capitalRepo repository.CapitalRepo
	Graph       *Graph
	Redis       *redis.Redis
	rankMap     map[string]int
}

func NewCapitalAnalysisBiz(db *gorm.DB, agg londobell.Agg, adapter londobell.Adapter, redis *redis.Redis) *CapitalAnalysisBiz {
	c := &CapitalAnalysisBiz{
		Agg:         agg,
		Adapter:     adapter,
		Redis:       redis,
		capitalRepo: prodal.NewCapitalDal(db),
		Graph:       NewGraph(),
	}
	once.Do(func() {
		now := time.Now()
		var err error
		err = c.loadRankMap()
		if err != nil {
			panic(fmt.Errorf("init rankMap failed: %w", err))
		}
		logger.Infof("init rankMap success")
		err = c.LoadData()
		if err != nil {
			panic(fmt.Errorf("init capital analysis graph failed: %w", err))
		}
		logger.Infof("init capital analysis graph success, total vertices cnt: %d, total edges cnt: %d", c.Graph.vertices, c.Graph.edges)
		logger.Infof("total init time: %s", time.Now().Sub(now).String())
	})
	go c.ReloadData()
	return c
}

func (c *CapitalAnalysisBiz) ReloadData() {
	cron := cron.New()
	// 添加定时任务
	err := cron.AddFunc("0 30 1 * * *", func() {
		c.Graph.Reset()
		err := c.LoadData()
		if err != nil {
			logger.Error(err)
		}
		err = c.loadRankMap()
		if err != nil {
			logger.Error(err)
		}
	}) // 每天1点30分执行任务，同步器1点获取数据
	if err != nil {
		return
	}
	// 启动定时任务
	cron.Start()
	// 阻塞主线程，保持定时任务运行
	select {}
}

func (c *CapitalAnalysisBiz) loadRankMap() (err error) {
	ctx := context.TODO()
	result, err := c.capitalRepo.GetAddressRank(ctx)
	if err != nil || result == nil {
		return
	}
	c.rankMap = make(map[string]int)
	for i, richAccount := range result.RichAccountRankList {
		c.rankMap[richAccount.Actor] = i + 1
	}
	return
}

func validateDataTime(entries []os.DirEntry) (err error) {
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() > entries[j].Name()
	}) //从大到小排序
	split := strings.Split(entries[0].Name(), "_")
	if len(split) != 0 {
		epoch := chain.CalcEpochByTime(time.Now()).CurrentDay().String()
		if split[0] != epoch {
			return fmt.Errorf("未能找到到最近一天的数据")
		}
	}
	if len(entries) != int(capital_task.Days.Int64()) {
		return fmt.Errorf("未能读取到%d天所有的数据", capital_task.Days)
	}
	return
}

// 导入所有的数据，建立map
func (c *CapitalAnalysisBiz) LoadData() (err error) {
	// 获取redis里的所有文件
	keys, err := c.Redis.Keys("*_transfer.json")
	if err != nil {
		logger.Errorf("get keys from redis error: %v", err)
		return
	}

	//  todo （记得加回来）做一个数据校验，是否包括7天的数据，且是从今天凌晨开始的前7天。（同步器能保证删除，所以保证数组长度为7即可）
	//err = validateDataTime(fileList)
	//if err != nil {
	//	logger.Error(err)
	//	return
	//}
	totalEdgeCnts := int64(0)
	// 遍历文件列表
	for _, key := range keys {
		if !strings.HasSuffix(key, "transfer.json") {
			continue
		}
		bytes, err := c.Redis.Get(key)
		if err != nil {
			logger.Errorf("无法读取文件 %s: %s\n", key, err)
		} else {
			//logger.Infof("文件 %s 内容:\n%s\n", filePath, string(content))
			edgeCnts := c.resolveFileContent(bytes)
			totalEdgeCnts += edgeCnts
			logger.Infof("加载文件- %s 成功", key)
		}
	}

	err = c.graphValidationAndSetEdges(totalEdgeCnts)
	if err != nil {
		logger.Error(err)
		return
	}
	c.initSortedSliceAndStatistics()
	//c.Graph.PrintGraph() //调试的时候可以使用，上线的时候关闭
	return
}

// 建立已经排序好的Slice
func (c *CapitalAnalysisBiz) initSortedSliceAndStatistics() {
	for _, list := range c.Graph.adjMap {
		var totalMap = make(map[string]*EdgeHead)
		var sortedSlice []*EdgeHead
		//如果是出入边为同一个的话，进行累加去重
		for k, v := range list.fromHeadMap { //加个独特的用户数量
			list.fromValue = list.fromValue.Add(v.totalVal)
			totalMap[k] = v
		}
		for k, v := range list.toHeadMap {
			list.toValue = list.toValue.Add(v.totalVal)
			if _, ok := totalMap[k]; ok {
				preV := totalMap[k]
				p := findTailPoint(preV.head)
				p.next = v.head
				totalMap[k] = &EdgeHead{ // 避免把原数据改掉
					to:        v.to,
					totalVal:  preV.totalVal.Add(v.totalVal),
					cnt:       preV.cnt + v.cnt,
					direction: "IN/OUT",
					head:      preV.head,
				}
			} else {
				totalMap[k] = v
			}
		}
		for _, v := range totalMap {
			sortedSlice = append(sortedSlice, v)
		}
		var tLen = len(sortedSlice)
		if tLen > 10 {
			tLen = 10
		}
		sort.Slice(sortedSlice, func(i, j int) bool {
			if sortedSlice[i].cnt != sortedSlice[j].cnt {
				return sortedSlice[i].cnt > sortedSlice[j].cnt
			} else {
				return sortedSlice[i].totalVal.GreaterThan(sortedSlice[j].totalVal)
			}
		})
		list.sortedSliceByCnt = make([]*EdgeHead, tLen)
		list.sortedSliceByVal = make([]*EdgeHead, tLen)
		copy(list.sortedSliceByCnt, sortedSlice[:tLen])
		sort.Slice(sortedSlice, func(i, j int) bool {
			if sortedSlice[i].totalVal.Equals(sortedSlice[j].totalVal) {
				return sortedSlice[i].cnt > sortedSlice[j].cnt
			} else {
				return sortedSlice[i].totalVal.GreaterThan(sortedSlice[j].totalVal)
			}
		})
		copy(list.sortedSliceByVal, sortedSlice[:tLen])
	}
}

func findTailPoint(p *Edge) (lastP *Edge) {
	pre := (*Edge)(nil)
	for p != nil {
		pre = p
		p = p.next
	}
	return pre
}

func (c *CapitalAnalysisBiz) graphValidationAndSetEdges(totalEdgeCnts int64) error {
	// 判断图的所有出入边是否相等
	totalToCnt := int64(0)
	totalFromCnt := int64(0)
	for _, list := range c.Graph.adjMap {
		totalFromCnt += list.fromCnt
		totalToCnt += list.toCnt
	}
	if totalFromCnt != totalToCnt {
		return fmt.Errorf("the total outgoing edges of a Graph are not equal to the total incoming edges")
	}
	if totalFromCnt != totalEdgeCnts {
		return fmt.Errorf("the total resolve edges of a Graph are not equal to the total edges")
	}
	c.Graph.edges = totalFromCnt
	return nil
}

// 解析文件，并将数据解析成边
func (c *CapitalAnalysisBiz) resolveFileContent(content []byte) (edgeCnts int64) {
	res := &londobell.TransferMessagesList{}
	err := json.Unmarshal(content, res)
	if err != nil {
		logger.Error("JSON 反序列化时发生了错误:", err)
	}
	for _, msg := range res.TransferMessages {
		c.Graph.AddEdge(msg.From.Address(), msg.To.Address(), msg.Epoch, msg.Cid, msg.Value)
	}
	return int64(len(res.TransferMessages))
}

func (c *CapitalAnalysisBiz) CapitalAddrInfo(ctx context.Context, req pro.AddrInfoRequest) (resp pro.AddrInfoResp, err error) {
	balance, proportion, err := c.getBalanceAndProportion(ctx, req.Address)
	if err != nil {
		if strings.Contains(err.Error(), "actor not found") {
			return resp, nil
		}
		return
	}
	resp.Balance = balance
	resp.Proportion = proportion
	if req.Address[1] != '0' {
		addrs := c.getAccountAddress(ctx, req.Address)
		req.Address = addrs[0]
	}
	if rank, ok := c.rankMap[req.Address]; ok {
		resp.Rank = rank
	} else {
		resp.Rank = -1
	}
	//计算余额的变化
	//var intervalObj interval.Interval
	//epoch, err := c.getEpoch(ctx)
	//if err != nil {
	//	return
	//}
	//intervalObj, err = interval.ResolveInterval(req.Interval, epoch)
	//if err != nil {
	//	return
	//}
	//startEpoch := intervalObj.Start()
	resp.Tag = convertor.GlobalTagMap[req.Address]
	//beforeBalance, err := c.capitalRepo.GetLatestBalanceBeforeEpoch(ctx, req.Address, &startEpoch)
	//if err != nil {
	//	if errors.Is(err, gorm.ErrRecordNotFound) {
	//		resp.BalanceIncrease = decimal.Zero
	//		// 没有其他要请求的内容了，直接返回结果
	//		return resp, nil
	//	} else {
	//		return
	//	}
	//}
	//afterBalance, err := c.capitalRepo.GetLatestBalanceAfterEpoch(ctx, req.Address, &startEpoch)
	//if err != nil {
	//	return
	//}
	//resp.BalanceIncrease = beforeBalance.Sub(afterBalance)
	accountAddress := c.getAccountAddress(ctx, req.Address)
	for _, address := range accountAddress {
		if c.Graph.adjMap[address] != nil {
			resp.BalanceIncrease = c.Graph.adjMap[address].fromValue.Add(c.Graph.adjMap[address].toValue)
			//resp.TotalTransactionCount = c.Graph.adjMap[address].fromCnt + c.Graph.adjMap[address].toCnt
			break
		}
	}
	return
}

func (c *CapitalAnalysisBiz) CapitalAddrTransaction(ctx context.Context, req pro.AddrTransactionRequest) (resp pro.AddrTransactionResp, err error) {
	balance, proportion, err := c.getBalanceAndProportion(ctx, req.Address)
	if err != nil {
		if strings.Contains(err.Error(), "actor not found") {
			return resp, mix.Warnf("the address you entered not found")
		}
		return resp, mix.Warnf(err.Error())
	}
	resp.Balance = balance
	resp.Proportion = proportion
	resp.Tag = convertor.GlobalTagMap[req.Address]
	accountAddress := c.getAccountAddress(ctx, req.Address)
	for _, address := range accountAddress {
		if c.Graph.adjMap[address] != nil {
			resp.TotalTransactionValue = c.Graph.adjMap[address].fromValue.Add(c.Graph.adjMap[address].toValue)
			resp.TotalTransactionCount = c.Graph.adjMap[address].fromCnt + c.Graph.adjMap[address].toCnt
			break
		}
	}
	return
}

func (c *CapitalAnalysisBiz) getBalanceAndProportion(ctx context.Context, addr string) (balance, proportion decimal.Decimal, err error) {
	totalFil := decimal.NewFromInt(2).Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(9)))
	totalFilAttoFil := totalFil.Mul(decimal.NewFromInt(10).Pow(decimal.NewFromInt(18)))

	epoch, err := c.getEpoch(ctx)
	if err != nil {
		return
	}
	actor, err := c.Adapter.Actor(ctx, chain.SmartAddress(addr), &epoch)
	if err != nil {
		return
	}
	balance = actor.Balance
	proportion = balance.Div(totalFilAttoFil)
	return
}

func (c *CapitalAnalysisBiz) getEpoch(ctx context.Context) (epoch chain.Epoch, err error) {
	r, err := c.Adapter.Epoch(ctx, nil)
	if err != nil {
		return
	}
	if r == nil {
		err = fmt.Errorf("query node epoch error: %s", err)
		return
	}
	epoch = chain.Epoch(r.Epoch)
	return
}

func (c *CapitalAnalysisBiz) EvaluateAddr(ctx context.Context, req pro.EvaluateAddrRequest) (resp pro.EvaluateAddrResp, err error) {
	if len(c.Graph.adjMap) == 0 {
		return resp, mix.Warnf("there is no data in the current graph")
	}
	if req.Type != "transaction_volume" && req.Type != "transaction_count" {
		return resp, mix.Warnf("there is wrong filter method")
	}
	address := c.getAccountAddress(ctx, req.Address)
	var node *pro.Node
	var nodes = make([]*pro.Node, 0)
	var edges = make([]*pro.Edge, 0)
	var nodesMap map[string]struct{}
	for _, addr := range address {
		//node = c.genLayer(addr, "", req.Type, 0)
		nodesMap = make(map[string]struct{})
		node = c.genGraphNode(addr, "", req.Type, &nodes, &edges, &nodesMap, 0)
		if node != nil {
			break
		}
	}
	for _, n := range nodes {
		accountAddress := c.getAccountAddress(ctx, n.Address)
		if accountAddress != nil {
			n.ShortAddress = accountAddress[0]
		}
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Level < nodes[j].Level
	})
	resp.Nodes = nodes
	resp.Edges = edges
	return
}

// 获取地址的其他地址信息
func (c *CapitalAnalysisBiz) getAccountAddress(ctx context.Context, addr string) (value []string) {
	value, ok := cache.Get(addr)
	if !ok {
		address, err := c.Agg.Address(ctx, chain.SmartAddress(addr))
		if err != nil {
			if strings.Contains(err.Error(), "error not found") {
				return []string{addr}
			}
			return nil
		}
		value = append(value, chain.SmartAddress(address.ActorID).Address())
		if address.RobustAddress != "" {
			value = append(value, chain.SmartAddress(address.RobustAddress).Address())
		}
		if address.DelegatedAddress != "" {
			value = append(value, chain.SmartAddress(address.DelegatedAddress).Address())
		}
		cache.Add(addr, value)
	}
	return value
}

func (c *CapitalAnalysisBiz) genLayer(addr, preAddr string, method string, depth int64) (node *pro.Node) {
	if depth > LAYERNUMS {
		return nil
	}
	adjList := c.Graph.adjMap[addr]
	if adjList == nil {
		return nil
	}
	n := &pro.Node{
		Address:                         addr,
		TotalTransactionVolume:          adjList.fromValue.Add(adjList.toValue),
		ToTransactionVolume:             adjList.toValue,
		FromTransactionVolume:           adjList.fromValue,
		UniqueToTransactionAddressCnt:   int64(len(adjList.toHeadMap)),
		UniqueFromTransactionAddressCnt: int64(len(adjList.fromHeadMap)),
		TotalCnt:                        adjList.toCnt + adjList.fromCnt,
		ToCnt:                           adjList.toCnt,
		FromCnt:                         adjList.fromCnt,
		Nodes:                           make([]*pro.Node, 0),
	}
	if depth == 0 {
		n.TotalTransactionVolume = adjList.fromValue.Add(adjList.toValue)
		n.TotalCnt = adjList.toCnt + adjList.fromCnt
	}
	var sortedSlice []*EdgeHead
	var widthNums = WIDTHNUMS
	if method == "transaction_volume" {
		sortedSlice = adjList.sortedSliceByVal
	} else if method == "transaction_count" {
		sortedSlice = adjList.sortedSliceByCnt
	}
	slen := len(sortedSlice)
	// 将和父类一致的地址给剔除掉
	for i := 0; i < slen; i++ {
		if sortedSlice[i].to == preAddr {
			sortedSlice = append(sortedSlice[:i], sortedSlice[i+1:]...)
			break
		}
	}
	if widthNums > len(sortedSlice) {
		widthNums = len(sortedSlice)
	}
	n.Width = int64(widthNums)
	for j := 0; j < widthNums; j++ {
		nextNode := c.genLayer(sortedSlice[j].to, addr, method, depth+1)
		if nextNode != nil {
			nextNode.TransactionVolumeWithFatherNode = sortedSlice[j].totalVal
			nextNode.CntWithFatherNode = sortedSlice[j].cnt
			nextNode.Level = depth + 1
			n.Nodes = append(n.Nodes, nextNode) //超过3层，返回为0，不加入了
		}
	}
	for _, n1 := range n.Nodes {
		n.CalChildCnt += n1.CntWithFatherNode
		n.CalChildTransactionVolume = n.CalChildTransactionVolume.Add(n1.TransactionVolumeWithFatherNode)
	}
	return n
}

func (c *CapitalAnalysisBiz) genGraphNode(addr, preAddr, method string, nodes *[]*pro.Node, edges *[]*pro.Edge, nodeMap *map[string]struct{}, depth int64) (node *pro.Node) {
	if depth > LAYERNUMS {
		return nil
	}
	adjList := c.Graph.adjMap[addr]
	if adjList == nil {
		return nil
	}
	n := &pro.Node{
		Address:                         addr,
		TotalTransactionVolume:          adjList.fromValue.Add(adjList.toValue),
		ToTransactionVolume:             adjList.toValue,
		FromTransactionVolume:           adjList.fromValue,
		UniqueToTransactionAddressCnt:   int64(len(adjList.toHeadMap)),
		UniqueFromTransactionAddressCnt: int64(len(adjList.fromHeadMap)),
		TotalCnt:                        adjList.toCnt + adjList.fromCnt,
		ToCnt:                           adjList.toCnt,
		FromCnt:                         adjList.fromCnt,
		Tag:                             convertor.GlobalTagMap[addr],
	}
	if depth == 0 {
		n.TotalTransactionVolume = adjList.fromValue.Add(adjList.toValue)
		n.TotalCnt = adjList.toCnt + adjList.fromCnt
	} else if _, ok := convertor.GlobalTagMap[addr]; ok { //第一层和交易所分开，即第0层为交易所要允许往下，其他情况不允许
		//交易所的点要显示，但是不能往下
		(*nodeMap)[addr] = struct{}{}
		*nodes = append(*nodes, n)
		return n
	}
	var sortedSlice []*EdgeHead
	var widthNums = WIDTHNUMS
	if method == "transaction_volume" {
		sortedSlice = make([]*EdgeHead, len(adjList.sortedSliceByVal))
		copy(sortedSlice, adjList.sortedSliceByVal) // 别污染系统的表
	} else if method == "transaction_count" {
		sortedSlice = make([]*EdgeHead, len(adjList.sortedSliceByCnt))
		copy(sortedSlice, adjList.sortedSliceByCnt)
	}
	slen := len(sortedSlice)
	//将和父类一致的地址给剔除掉
	for i := 0; i < slen; i++ {
		if sortedSlice[i].to == preAddr {
			sortedSlice = append(sortedSlice[:i], sortedSlice[i+1:]...)
			break
		}
	}
	if widthNums > len(sortedSlice) {
		widthNums = len(sortedSlice)
	}
	n.Width = int64(widthNums)
	tmpChildNodes := make([]*pro.Node, 0)
	(*nodeMap)[n.Address] = struct{}{}
	for j := 0; j < widthNums; j++ {
		if !c.isRedundant(*nodeMap, sortedSlice[j].to) {
			c.addEdges(edges, addr, sortedSlice[j])
		}
	} //优化上层节点前端的显示，层次+深度遍历结合，不影响结果(当图中出现环时，按层次优先展示关系)
	for j := 0; j < widthNums; j++ {
		if c.isRedundant(*nodeMap, sortedSlice[j].to) {
			//跳过这个，显示不用变少，点是点的逻辑，边可以全连上
			continue
		}
		nextNode := c.genGraphNode(sortedSlice[j].to, addr, method, nodes, edges, nodeMap, depth+1)
		if nextNode != nil {
			nextNode.TransactionVolumeWithFatherNode = sortedSlice[j].totalVal
			nextNode.CntWithFatherNode = sortedSlice[j].cnt
			nextNode.Level = depth + 1
			n.CalChildCnt += nextNode.CntWithFatherNode
			n.CalChildTransactionVolume = n.CalChildTransactionVolume.Add(nextNode.TransactionVolumeWithFatherNode)
			tmpChildNodes = append(tmpChildNodes, nextNode)
		}
	}
	for _, childNode := range tmpChildNodes {
		if method == "transaction_volume" {
			childNode.ProportionWithFatherNode = childNode.TransactionVolumeWithFatherNode.Div(n.CalChildTransactionVolume)
		} else if method == "transaction_count" {
			childNode.ProportionWithFatherNode = decimal.NewFromInt(childNode.CntWithFatherNode).Div(decimal.NewFromInt(n.CalChildCnt))
		}
	}
	*nodes = append(*nodes, n)
	return n
}

func (c *CapitalAnalysisBiz) addEdges(edges *[]*pro.Edge, addr string, edge *EdgeHead) {
	var e *pro.Edge
	var head *EdgeHead
	var direction = edge.direction
	if edge.direction == "OUT" {
		head = c.Graph.adjMap[addr].toHeadMap[edge.to]
	} else if edge.direction == "IN" {
		head = c.Graph.adjMap[addr].fromHeadMap[edge.to]
	} else {
		head = edge
	}
	e = &pro.Edge{
		From:      addr,
		To:        edge.to,
		TotalVal:  edge.totalVal,
		TotalCnt:  edge.cnt,
		Direction: direction,
	}

	//head := c.Graph.adjMap[from].toHeadMap[to] //这里可能是from和to的组合
	if head != nil {
		p := head.head
		for p != nil {
			subE := &pro.SubEdge{
				Cid:          p.cid,
				Value:        p.value,
				Epoch:        p.epoch,
				ExchangeHour: p.exchangeHour,
				Direction:    p.direction,
			}
			e.Details = append(e.Details, subE)
			p = p.next
		}
	}
	*edges = append(*edges, e)
}

// 去重函数，判断已经存在了，即不再进行添加节点
func (c *CapitalAnalysisBiz) isRedundant(nodeMap map[string]struct{}, addr string) bool {
	if _, ok := nodeMap[addr]; ok {
		return true
	}
	return false
}
