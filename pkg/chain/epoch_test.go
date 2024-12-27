package chain

import (
	"testing"
	"time"
	
	"github.com/golang-module/carbon/v2"
	"github.com/stretchr/testify/require"
)

func TestCalcEpochByTime(t *testing.T) {
	
	ts, err := time.Parse(LayoutStyle, "2023-09-01 00:00:00")
	require.NoError(t, err)
	t.Log(CalcEpochByTime(ts))
	
	ts, err = time.Parse(LayoutStyle, "2023-05-19 00:00:00")
	require.NoError(t, err)
	t.Log(CalcEpochByTime(ts))
	
	ts, err = time.Parse(LayoutStyle, "2023-01-29 00:00:00")
	require.NoError(t, err)
	t.Log(CalcEpochByTime(ts))
	
	ts, err = time.Parse(LayoutStyle, "2022-12-17 00:00:00")
	require.NoError(t, err)
	t.Log(CalcEpochByTime(ts))
	
	ts, err = time.Parse(LayoutStyle, "2022-12-31 00:00:00")
	require.NoError(t, err)
	t.Log(CalcEpochByTime(ts))
	
	//ts, err = time.Parse(LayoutStyle, "2023-01-02 00:00:00")
	//require.NoError(t, err)
	//t.Log(CalcEpochByTime(ts))
	//
	//ts, err = time.Parse(LayoutStyle, "2020-08-27 00:00:00")
	//require.NoError(t, err)
	//t.Log(CalcEpochByTime(ts))
	//
	//ts, err = time.Parse(LayoutStyle, "2021-04-16 00:00:00")
	//require.NoError(t, err)
	//t.Log(CalcEpochByTime(ts))
	//
	//ts, err = time.Parse(LayoutStyle, "2021-07-19 00:00:00")
	//require.NoError(t, err)
	//t.Log(CalcEpochByTime(ts))
	//
	//ts, err = time.Parse(LayoutStyle, "2022-09-01 00:00:00")
	//require.NoError(t, err)
	//t.Log(CalcEpochByTime(ts))
	
}

func TestCalcTimeByEpoch(t *testing.T) {
	b, err := time.ParseInLocation(carbon.DateTimeLayout, "2023-05-04 14:05:00", TimeLoc)
	require.NoError(t, err)
	RegisterBaseTime(528464, b)
	ts, err := time.Parse(carbon.DateTimeLayout, "2023-03-06 09:29:30")
	require.NoError(t, err)
	t.Log(CalcEpochByTime(ts))
	
}

func TestCalcEpochDay(t *testing.T) {
	t.Log(CurrentEpoch())
	t.Log(CalcTimeByEpoch(2160))
	t.Log(Epoch(3028214).Format())
}

func TestEpochStartOfDay(t *testing.T) {
	require.Equal(t, "2020-08-25", Epoch(0).CurrentDay().Date())
	require.Equal(t, "2020-08-26", Epoch(2160).CurrentDay().Date())
	require.Equal(t, "2020-08-27", Epoch(5040).CurrentDay().Date())
}

//func TestEpochDays(t *testing.T) {
//	t.Log(Epoch(0).ElapsedDays(), Epoch(0).ElapsedDays().ZeroEpoch())
//	t.Log(Epoch(2160).ElapsedDays(), Epoch(2160).ElapsedDays().ZeroEpoch())
//	t.Log(Epoch(5040).ElapsedDays(), Epoch(5040).ElapsedDays().ZeroEpoch())
//}

func TestEpochReleaseReward(t *testing.T) {
	//epoch := Epoch(1689840)
	//last := epoch.LastReleaseRewardEpoch()
	//require.Equal(t, epoch, last.RewardEpoch())
	t.Log(Epoch(3029087).String())
}

func TestEpochStartOfDay2(t *testing.T) {
	epoch := Epoch(1689840)
	t.Log(epoch.Format())
	d, err := BuildEpochByTime(epoch.Format())
	require.NoError(t, err)
	t.Log(d.Format())
	require.Equal(t, epoch.Format(), epoch.CurrentDay().Format())
	
	//v := abi.NewTokenAmount(0)
	//spew.Json(v)
}
