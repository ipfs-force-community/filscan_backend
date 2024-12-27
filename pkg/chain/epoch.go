package chain

import (
	"fmt"
	"time"
)

const (
	LayoutStyle = "2006-01-02 15:04:05"
)

func RegisterBaseTime(epoch int64, blockTime time.Time) {
	baseTime = time.Unix(blockTime.Unix()-epoch*30, 0)
}

var (
	beginTime   = "2020-08-25 06:00:00" // 高度0时的时间
	TimeLoc, _  = time.LoadLocation("Asia/Shanghai")
	baseTime, _ = time.ParseInLocation(LayoutStyle, beginTime, TimeLoc)
)

func MustBuildEpochByDate(date string) Epoch {
	t, err := time.ParseInLocation(LayoutStyle, fmt.Sprintf("%s 00:00:00", date), TimeLoc)
	if err != nil {
		panic(err)
	}
	return CalcEpochByTime(t)
}

func BuildEpochByDate(date string) (epoch Epoch, err error) {
	t, err := time.ParseInLocation(LayoutStyle, fmt.Sprintf("%s 00:00:00", date), TimeLoc)
	if err != nil {
		return
	}
	epoch = CalcEpochByTime(t)
	return
}

func MustBuildEpochByTime(t string) Epoch {
	r, err := BuildEpochByTime(t)
	if err != nil {
		panic(err)
	}
	return r
}

func BuildEpochByTime(t string) (epoch Epoch, err error) {
	tz, err := time.ParseInLocation(LayoutStyle, t, TimeLoc)
	if err != nil {
		return
	}
	if tz.Before(baseTime) {
		return
	}
	epoch = CalcEpochByTime(tz)
	return
}

// LCRORange 左闭右开高度区间
type LCRORange struct {
	GteBegin Epoch
	LtEnd    Epoch
}

func NewLCRORange(gteBegin Epoch, ltEnd Epoch) LCRORange {
	return LCRORange{GteBegin: gteBegin, LtEnd: ltEnd}
}

func (l LCRORange) Valid() error {
	if l.GteBegin >= l.LtEnd {
		return fmt.Errorf("无效高度区间: [%d,%d)", l.GteBegin, l.LtEnd)
	}
	return nil
}

type Epoch int64

func (e Epoch) CurrentHour() Epoch {
	return e / 120 * 120
}

func (e Epoch) CurrentDay() Epoch {
	r, _ := BuildEpochByDate(e.Date())
	return r
}

func (e Epoch) Int64() int64 {
	return int64(e)
}

func (e Epoch) BlockTime() BlockTime {
	return CalcTimeByEpoch(int64(e))
}

func (e Epoch) Time() time.Time {
	return CalcTimeByEpoch(int64(e)).Time()
}

func (e Epoch) Unix() int64 {
	return e.Time().Unix()
}

// Date 取北京时间的日期
func (e Epoch) Date() string {
	return e.BlockTime().Time().Format("2006-01-02")
}

// Format 取北京时间的日期时间
func (e Epoch) Format() string {
	return e.BlockTime().Time().Format(LayoutStyle)
}

func (e Epoch) Next() Epoch {
	return e + 1
}

func (e Epoch) String() string {
	return fmt.Sprintf("%d(%s)", e, e.Format())
}

// RewardEpoch 计算以 e 高度为最后一个释放奖励高度对应产生爆块的高度
// 如果结果小于 0，则返回 0
func (e Epoch) RewardEpoch() Epoch {
	// 爆块奖励自产生高度算起，会在 180 天后的对应高度释放完毕
	r := e - 518400
	if r <= 0 {
		return 0
	}
	return r
}

// LastReleaseRewardEpoch  计算以 e 产生爆块，最后一个释放奖励高度
func (e Epoch) LastReleaseRewardEpoch() Epoch {
	return e + 518400
}

// ElapsedDays 距离 0 高度过去多少天
func (e Epoch) ElapsedDays() ElapsedDays {
	return ElapsedDays((e + 720) / 2880)
}

type ElapsedDays int64

func NewLCRCRange(gteBegin Epoch, lteEnd Epoch) LCRCRange {
	return LCRCRange{GteBegin: gteBegin, LteEnd: lteEnd}
}

// LCRCRange 左闭右闭区间
type LCRCRange struct {
	GteBegin Epoch
	LteEnd   Epoch
}

func (e LCRCRange) Valid() error {
	if e.LteEnd < e.GteBegin {
		return fmt.Errorf("epoch_range expect end > beigin, got: %d,%d", e.GteBegin, e.LteEnd)
	}
	return nil
}

func NewLORCRange(gtBegin Epoch, lteEnd Epoch) LORCRange {
	return LORCRange{GtBegin: gtBegin, LteEnd: lteEnd}
}

// LORCRange 左开右闭区间
type LORCRange struct {
	GtBegin Epoch
	LteEnd  Epoch
}

func (e LORCRange) Valid() error {
	if e.LteEnd < e.GtBegin {
		return fmt.Errorf("epoch_range expect end > beigin, got: %d,%d", e.GtBegin, e.LteEnd)
	}
	return nil
}

type BlockTime time.Time

func (b BlockTime) Epoch() Epoch {
	return CalcEpochByTime(time.Time(b))
}

func (b BlockTime) String() string {
	return time.Time(b).String()
}

func (b BlockTime) Time() time.Time {
	return time.Time(b)
}

// FromWeekOfYearToMondayAndSunday 根据一年所在的第N周计算出这周的周一和周日的日期。
// 日期取北京时间东八区。
func FromWeekOfYearToMondayAndSunday(weeks int) (monday, sunday time.Time) {
	// 计算原理：
	// 根据当年的日期计算出当年的1月1日，根据1月1日所在的周数看看是否是每年的第一周
	// 如果不是每年的第一周，计算下一周的周一。下一周的周一即每年的第一周。再用总周数求出总周数开始的周一和周日
	now := time.Now()
	baseDate := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, TimeLoc)
	year, week := baseDate.ISOWeek()
	wd := int(baseDate.Weekday())
	if year != now.Year() && week > 1 {
		if wd == 0 {
			baseDate = baseDate.AddDate(0, 0, 1)
		} else {
			baseDate = baseDate.AddDate(0, 0, 7-wd+1)
		}
	} else if wd != 1 {
		baseDate = baseDate.AddDate(0, 0, -wd+1)
	}
	
	monday = baseDate.Add(time.Duration(weeks-1) * 7 * 24 * time.Hour)
	sunday = monday.AddDate(0, 0, 6)
	return
}

// CalcWeeks 根据传入的时间，计算出该时间所处全年所在的第N周。
// 时间取北京时间东八区。
func CalcWeeks(ts time.Time) (year, weeks int) {
	return ts.In(TimeLoc).ISOWeek()
}

func CurrentEpoch() Epoch {
	return CalcEpochByTime(time.Now().In(TimeLoc))
}

func CalcEpochByTime(tz time.Time) Epoch {
	tz, _ = time.ParseInLocation(LayoutStyle, tz.Format(LayoutStyle), TimeLoc)
	if tz.Before(baseTime) {
		return 0
	}
	return Epoch((tz.Unix() - baseTime.Unix()) / 30)
}

func CalcTimeByEpoch(height int64) BlockTime {
	return BlockTime(time.Unix(baseTime.Unix()+int64(height)*30, 0).In(TimeLoc))
}

// NextEpochInterval 下一个高度间隔时间
func NextEpochInterval() time.Duration {
	second := time.Now().Second()
	if second < 30 {
		second += 30
	}
	return time.Duration(60-second) * time.Second
}
