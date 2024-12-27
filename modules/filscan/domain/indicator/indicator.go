package indicator

import (
	"fmt"
	"gitlab.forceup.in/fil-data-factory/filscan-backend/pkg/chain"
)

/*
 TODO 未进行单元测试
*/

type RangeType string

const (
	RangeType_Day   RangeType = "day"
	RangeType_Week  RangeType = "week"
	RangeType_Month RangeType = "month"
	RangeType_Year  RangeType = "year"
)

func Resolve(t RangeType, end chain.Epoch) (Range, error) {
	switch t {
	case RangeType_Day:
		return &Day{end: end}, nil
	case RangeType_Week:
		return &Week{end: end}, nil
	case RangeType_Month:
		return &Month{end: end}, nil
	case RangeType_Year:
		return &Year{end: end}, nil
	}
	return nil, fmt.Errorf("unkonwon range type: %s", t)
}

type Range interface {
	Name() string
	Range() chain.LCRORange
}

var _ Range = (*Day)(nil)

type Day struct {
	end chain.Epoch
}

func (d Day) Name() string {
	return "24H"
}

func (d Day) Range() chain.LCRORange {
	return chain.NewLCRORange(d.end-2880, d.end)
}

var _ Range = (*Week)(nil)

type Week struct {
	end chain.Epoch
}

func (d Week) Name() string {
	return "7天"
}

func (d Week) Range() chain.LCRORange {
	return chain.NewLCRORange(d.end-2880*7, d.end)
}

var _ Range = (*Month)(nil)

type Month struct {
	end chain.Epoch
}

func (d Month) Name() string {
	return "30天"
}

func (d Month) Range() chain.LCRORange {
	return chain.NewLCRORange(d.end-2880*30, d.end)
}

var _ Range = (*Year)(nil)

type Year struct {
	end chain.Epoch
}

func (d Year) Name() string {
	return "1年"
}

func (d Year) Range() chain.LCRORange {
	return chain.NewLCRORange(d.end-2880*365, d.end)
}
