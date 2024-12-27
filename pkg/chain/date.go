package chain

import (
	"encoding/json"
	"github.com/golang-module/carbon/v2"
	"time"
)

func NewDate(tz, ts string) (date Date, err error) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return
	}
	
	t, err := time.ParseInLocation("2006-01-02", ts, loc)
	if err != nil {
		return
	}
	
	date = Date{
		time: t,
		loc:  loc,
	}
	return
}

var _ json.Marshaler = (*Date)(nil)

// 定义日期格式: 2006-01-02
type Date struct {
	time time.Time
	loc  *time.Location
}

func (d Date) MarshalJSON() ([]byte, error) {
	return d.time.MarshalJSON()
}

func (d Date) calcEpochByTimeWithTimezone(loc *time.Location, t time.Time) Epoch {
	t, _ = time.ParseInLocation(LayoutStyle, t.Format(LayoutStyle), loc)
	if t.Before(baseTime) {
		return 0
	}
	return Epoch((t.Unix() - baseTime.Unix()) / 30)
}

// 判断当前日期是否是今天
func (d Date) IsToday(loc *time.Location) (ok bool) {
	return d.Format() == time.Now().In(loc).Format(carbon.DateLayout)
}

func (d Date) Lt(t Date) bool {
	return d.time.Before(t.time)
}

func (d Date) Gt(t Date) bool {
	return d.time.After(t.time)
}

func (d Date) Gte(t Date) bool {
	return d.time.After(t.time) || d.Format() == t.Format()
}

func (d Date) Lte(t Date) bool {
	return d.time.Before(t.time) || d.Format() == t.Format()
}

func (d Date) AddDay() Date {
	return Date{
		time: d.time.AddDate(0, 0, 1),
		loc:  d.loc,
	}
}

func (d Date) SubDay() Date {
	return Date{
		time: d.time.AddDate(0, 0, -1),
		loc:  d.loc,
	}
}

func (d Date) Epoch() Epoch {
	return d.calcEpochByTimeWithTimezone(d.loc, d.time)
}

func (d Date) NextDay() Epoch {
	return d.calcEpochByTimeWithTimezone(d.loc, d.time.AddDate(0, 0, 1))
}

func (d Date) epochs() LCRORange {
	return LCRORange{
		GteBegin: d.Epoch(),
		LtEnd:    d.NextDay(),
	}
}

func (d Date) SafeEpochs() LCRORange {
	epochs := d.epochs()
	current := d.currentEpochWithLoc()
	if epochs.LtEnd > current {
		epochs.LtEnd = current
	}
	return epochs
}

func (d Date) Time() time.Time {
	return d.time
}

func (d Date) Format() string {
	t := d.Time()
	return t.Format(carbon.DateLayout)
}

func (d Date) currentEpochWithLoc() Epoch {
	return d.calcEpochByTimeWithTimezone(d.loc, time.Now().In(d.loc))
}

func NewDateLCRCRange(gteBegin Date, lteEnd Date) DateLCRCRange {
	return DateLCRCRange{GteBegin: gteBegin, LteEnd: lteEnd}
}

type DateLCRCRange struct {
	GteBegin Date
	LteEnd   Date
}
