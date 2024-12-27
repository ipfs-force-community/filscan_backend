package probiz

import (
	"fmt"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/shopspring/decimal"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

func NewGraph() *Graph {
	graph := &Graph{
		vertices: 0,
		edges:    0,
		adjMap:   make(map[string]*AdjList),
	}
	cache = expirable.NewLRU[string, []string](5000, nil, time.Hour*12)
	return graph
}

type Graph struct {
	vertices int64
	edges    int64
	adjMap   map[string]*AdjList // 邻接表map
}

type AdjList struct {
	toCnt            int64
	fromCnt          int64
	toValue          decimal.Decimal
	fromValue        decimal.Decimal
	toHeadMap        map[string]*EdgeHead
	fromHeadMap      map[string]*EdgeHead
	sortedSliceByCnt []*EdgeHead
	sortedSliceByVal []*EdgeHead
}

type EdgeHead struct {
	to        string
	totalVal  decimal.Decimal
	cnt       int64
	head      *Edge
	direction string
}

type Edge struct {
	cid          string
	value        decimal.Decimal
	next         *Edge
	epoch        int64
	exchangeHour int64
	direction    string
}

func (graph *Graph) Reset() {
	graph.adjMap = make(map[string]*AdjList)
	graph.vertices = 0
	graph.edges = 0
}

func (graph *Graph) AddEdge(source string, destination string, epoch int64, cid string, value decimal.Decimal) {
	// 检查源节点、目标节点是否存在，如果不存在则创建节点
	if _, ok := graph.adjMap[source]; !ok {
		graph.adjMap[source] = &AdjList{}
		graph.adjMap[source].fromHeadMap = make(map[string]*EdgeHead)
		graph.adjMap[source].toHeadMap = make(map[string]*EdgeHead)
		graph.vertices++
	}
	if _, ok := graph.adjMap[destination]; !ok {
		graph.adjMap[destination] = &AdjList{}
		graph.adjMap[destination].fromHeadMap = make(map[string]*EdgeHead)
		graph.adjMap[destination].toHeadMap = make(map[string]*EdgeHead)
		graph.vertices++
	}
	// 如果头节点不存在
	if _, ok := graph.adjMap[source].toHeadMap[destination]; !ok {
		graph.adjMap[source].toHeadMap[destination] = &EdgeHead{
			to:        destination,
			totalVal:  decimal.Zero,
			cnt:       0,
			head:      nil,
			direction: "OUT",
		}
	}
	if _, ok := graph.adjMap[destination].fromHeadMap[source]; !ok {
		graph.adjMap[destination].fromHeadMap[source] = &EdgeHead{
			to:        source,
			totalVal:  decimal.Zero,
			cnt:       0,
			head:      nil,
			direction: "IN",
		}
	}
	// 创建新的边节点
	newEdge := &Edge{
		cid:          cid,
		value:        value,
		epoch:        epoch,
		exchangeHour: chain.Epoch(epoch).Unix(),
		next:         graph.adjMap[source].toHeadMap[destination].head,
		direction:    "OUT",
	}

	// 更新出边头节点指针和出边数量
	graph.adjMap[source].toHeadMap[destination].head = newEdge
	graph.adjMap[source].toHeadMap[destination].cnt++
	graph.adjMap[source].toCnt++
	graph.adjMap[source].toHeadMap[destination].totalVal = graph.adjMap[source].toHeadMap[destination].totalVal.Add(value)

	// 创建新的反向边节点
	newReverseEdge := &Edge{
		cid:          cid,
		value:        value,
		epoch:        epoch,
		exchangeHour: chain.Epoch(epoch).Unix(),
		next:         graph.adjMap[destination].fromHeadMap[source].head,
		direction:    "IN",
	}
	// 更新入边头节点指针和入边数量
	graph.adjMap[destination].fromHeadMap[source].head = newReverseEdge
	graph.adjMap[destination].fromHeadMap[source].cnt++
	graph.adjMap[destination].fromCnt++
	graph.adjMap[destination].fromHeadMap[source].totalVal = graph.adjMap[destination].fromHeadMap[source].totalVal.Add(value)
}

func (graph *Graph) NumsOfVertices() int64 {
	return graph.vertices
}

func (graph *Graph) NumsOfEdges() int64 {
	return graph.edges
}

// 打印图的邻接表表示
func (graph *Graph) PrintGraph() {
	for vertex, adjList := range graph.adjMap {
		fmt.Printf("邻接表节点 %s\n", vertex)
		fmt.Printf("出边数量: %d\n", adjList.toCnt)
		fmt.Printf("出边: \n")
		for key, head := range adjList.toHeadMap {
			fmt.Printf("object: %s, total cnt: %d, total value: %s", key, head.cnt, head.totalVal.String())
			//tempTo := head.head
			//for tempTo != nil {
			//	// 这里可以关于cid 和 value todo 可拓展
			//	fmt.Printf("-> (value: %s) ", tempTo.value.String())
			//	tempTo = tempTo.next
			//}
			fmt.Println("")
		}
		fmt.Printf("入边数量: %d\n", adjList.fromCnt)
		fmt.Printf("入边: \n")
		for key, head := range adjList.fromHeadMap {
			fmt.Printf("object: %s, total cnt: %d, total value: %s", key, head.cnt, head.totalVal.String())
			//tempFrom := head.head
			//for tempFrom != nil {
			//	fmt.Printf("<- (value: %s) ", tempFrom.value.String())
			//	tempFrom = tempFrom.next
			//}
			fmt.Println("")
		}
		fmt.Println()
	}
}
