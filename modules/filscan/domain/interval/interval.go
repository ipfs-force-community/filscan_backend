package interval

import (
	"fmt"
	"sort"
	"strconv"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

const (
	hour  Type = "h"
	day   Type = "d"
	month Type = "m"
	year  Type = "y"
)

type Type string

// ResolveType 解析时间间隔字符串
// 24h => 24, h
// 7d  => 7, d
// 12m  => 12, m
func ResolveType(s string) (gap int, t Type, err error) {

	l := len(s)

	if l < 2 {
		err = fmt.Errorf("invaild interval string: %s", s)
		return
	}

	g, err := strconv.ParseInt(s[0:l-1], 10, 64)
	if err != nil {
		return
	}
	gap = int(g)
	v := Type(s[l-1:])
	switch v {
	case hour, day, month, year:
		t = v
	default:
		err = fmt.Errorf("invaild interval type: %s", s)
		return
	}
	return
}

// ResolveInterval 适配不同的统计间隔类型
func ResolveInterval(s string, current chain.Epoch) (Interval, error) {
	gap, t, err := ResolveType(s)
	if err != nil {
		return nil, err
	}
	switch t {
	case hour:
		return NewHours(gap, current), nil
	case day:
		return NewDays(gap, current), nil
	case month:
		return NewMonths(gap, current), nil
	case year:
		return NewYears(gap, current), nil
	}
	return nil, fmt.Errorf("unspoorted interval type: %s", t)
}

func ResolveInterval2(s string, current chain.Epoch) (Interval, error) {
	gap, t, err := ResolveType(s)
	if err != nil {
		return nil, err
	}
	switch t {
	case hour:
		return NewHours(gap, current), nil
	case day:
		return NewDays(gap, current), nil
	case month:
		return NewMonths2(gap, current), nil
	case year:
		return NewYears(gap, current), nil
	}
	return nil, fmt.Errorf("unspoorted interval type: %s", t)
}

type Interval interface {
	Type() string
	Gap() int
	Current() chain.Epoch
	Points() []chain.Epoch // 原理：如 24H, 从最新的高度按时间间隔往前推，得到 25 个点，前 24 个点用来返回，最后 1 个点用来计算比较值
	Start() chain.Epoch
}

// ToHourlyPoint 在耗时的数据库操作时候，减少采点。提高响应速度
func ToHourlyPoint(epochs []chain.Epoch) []chain.Epoch {
	reducedEpoch := make([]chain.Epoch, 0)
	epochMap := make(map[chain.Epoch]struct{})
	for _, epoch := range epochs {
		lepoch := epoch - epoch%120
		epochMap[lepoch] = struct{}{}
	}
	for epoch := range epochMap {
		reducedEpoch = append(reducedEpoch, epoch)
	}
	sort.Slice(reducedEpoch, func(i, j int) bool {
		return reducedEpoch[i].Int64() < reducedEpoch[j].Int64()
	})
	return reducedEpoch
}

func NewHours(gap int, current chain.Epoch) *Hours {
	return &Hours{gap: gap, epoch: current}
}

var _ Interval = (*Hours)(nil)

type Hours struct {
	gap   int
	epoch chain.Epoch
}

func (l Hours) Start() chain.Epoch {
	return l.epoch - chain.Epoch(l.gap*120)
}

func (l Hours) Gap() int {
	return l.gap
}

func (l Hours) Type() string {
	return fmt.Sprintf("%d%s", l.gap, hour)
}

func (l Hours) Current() chain.Epoch {
	return l.epoch
}

func (l Hours) Points() []chain.Epoch {
	var epochs []chain.Epoch
	start := l.epoch - chain.Epoch(l.gap*120)
	for i := start; i <= l.epoch; i += 60 {
		epochs = append(epochs, i)
	}
	return epochs
}

func NewDays(gap int, current chain.Epoch) *Days {
	return &Days{gap: gap, epoch: current}
}

var _ Interval = (*Days)(nil)

type Days struct {
	gap   int
	epoch chain.Epoch
}

func (l Days) Start() chain.Epoch {
	return l.epoch - chain.Epoch(l.gap*2880)
}

func (l Days) Gap() int {
	return l.gap
}

func (l Days) Type() string {
	return fmt.Sprintf("%d%s", l.gap, day)
}

func (l Days) Current() chain.Epoch {
	return l.epoch
}

func (l Days) Points() []chain.Epoch {
	var epochs []chain.Epoch
	start := l.epoch - chain.Epoch(l.gap*2880)
	for i := start; i <= l.epoch; i = i + 120 {
		epochs = append(epochs, i)
	}
	return epochs
}

func NewMonths(gap int, current chain.Epoch) *Months {
	return &Months{gap: gap, epoch: current.CurrentDay()}
}

func NewMonths2(gap int, current chain.Epoch) *Months2 {
	return &Months2{gap: gap, epoch: current.CurrentDay(), real: current}
}

var _ Interval = (*Months)(nil)

type Months struct {
	gap   int
	epoch chain.Epoch
}

func (l Months) Start() chain.Epoch {
	return l.epoch - chain.Epoch(l.gap*30*2880)
}

func (l Months) Gap() int {
	return l.gap
}

func (l Months) Type() string {
	return fmt.Sprintf("%d%s", l.gap, month)
}

func (l Months) Current() chain.Epoch {
	return l.epoch
}

func (l Months) Points() []chain.Epoch {
	var epochs []chain.Epoch
	start := l.epoch - chain.Epoch(l.gap*30*2880)
	for i := start; i <= l.epoch; i += 2880 {
		epochs = append(epochs, i)
	}
	return epochs
}

var _ Interval = (*Years)(nil)

func NewYears(gap int, current chain.Epoch) *Years {
	return &Years{gap: gap, epoch: current.CurrentDay()}
}

type Years struct {
	gap   int
	epoch chain.Epoch
}

func (l Years) Start() chain.Epoch {
	return l.epoch.CurrentDay() - chain.Epoch(l.gap*365*2880)
}

func (l Years) Gap() int {
	return l.gap
}

func (l Years) Type() string {
	return fmt.Sprintf("%d%s", l.gap, year)
}

func (l Years) Current() chain.Epoch {
	return l.epoch.CurrentDay()
}

func (l Years) Points() []chain.Epoch {
	var epochs []chain.Epoch
	start := l.Start()
	for i := start; i < l.epoch; i += 2880 * 30 {
		epochs = append(epochs, i)
	}
	// 给出当天实时数据
	epochs = append(epochs, l.epoch)
	return epochs
}

type Months2 struct {
	gap   int
	epoch chain.Epoch
	real  chain.Epoch
}

func (l Months2) Start() chain.Epoch {
	return l.epoch - chain.Epoch(l.gap*30*2880)
}

func (l Months2) Gap() int {
	return l.gap
}

func (l Months2) Type() string {
	return fmt.Sprintf("%d%s", l.gap, month)
}

func (l Months2) Current() chain.Epoch {
	return l.epoch
}

func (l Months2) Points() []chain.Epoch {
	var epochs []chain.Epoch
	start := l.epoch - chain.Epoch(l.gap*30*2880)
	for i := start; i <= l.epoch; i += 2880 {
		epochs = append(epochs, i)
	}

	epochs = append(epochs, l.real)
	return epochs
}
