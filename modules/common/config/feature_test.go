package config

import "testing"

func boolPtr(v bool) *bool    { return &v }
func int64Ptr(v int64) *int64 { return &v }

// 开关默认必须全关：老配置文件（完全没有 [feature] 段）不能因为改造而改变行为，也不能 panic。
func TestFeatureSwitchesDefaultOff(t *testing.T) {
	cases := []struct {
		name string
		conf *Config
	}{
		{"nil config", nil},
		{"no feature section", &Config{}},
		{"empty feature section", &Config{Feature: &Feature{}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if c.conf.MinerBlockRewardReadFromPg() {
				t.Error("MinerBlockRewardReadFromPg 默认应为 false")
			}
			if c.conf.MinersBlockRewardReadFromPg() {
				t.Error("MinersBlockRewardReadFromPg 默认应为 false")
			}
			if c.conf.MinerWinCountReadFromPg() {
				t.Error("MinerWinCountReadFromPg 默认应为 false")
			}
			if got := c.conf.PgReadTimeoutMs(); got != PgReadDefaultTimeoutMs {
				t.Errorf("PgReadTimeoutMs 默认应为 %d，得到 %d", PgReadDefaultTimeoutMs, got)
			}
		})
	}
}

func TestFeatureSwitchesOn(t *testing.T) {
	conf := &Config{Feature: &Feature{
		MinerBlockRewardReadFromPg:  boolPtr(true),
		MinersBlockRewardReadFromPg: boolPtr(false),
		MinerWinCountReadFromPg:     boolPtr(true),
		PgReadTimeoutMs:             int64Ptr(1500),
	}}
	if !conf.MinerBlockRewardReadFromPg() {
		t.Error("MinerBlockRewardReadFromPg 应为 true")
	}
	if conf.MinersBlockRewardReadFromPg() {
		t.Error("MinersBlockRewardReadFromPg 应为 false")
	}
	if !conf.MinerWinCountReadFromPg() {
		t.Error("MinerWinCountReadFromPg 应为 true")
	}
	if got := conf.PgReadTimeoutMs(); got != 1500 {
		t.Errorf("PgReadTimeoutMs 应为 1500，得到 %d", got)
	}
}

// 0 = 用默认（避免把漏配当永不超时）；负数 = 显式不设超时（由调用方解释）。
func TestPgReadTimeoutSemantics(t *testing.T) {
	if got := (&Config{Feature: &Feature{PgReadTimeoutMs: int64Ptr(0)}}).PgReadTimeoutMs(); got != PgReadDefaultTimeoutMs {
		t.Errorf("配置 0 应回落到默认 %d，得到 %d", PgReadDefaultTimeoutMs, got)
	}
	if got := (&Config{Feature: &Feature{PgReadTimeoutMs: int64Ptr(-1)}}).PgReadTimeoutMs(); got != -1 {
		t.Errorf("配置负数应原样返回，得到 %d", got)
	}
}
