package main

import (
	"reflect"
	"strings"
	"testing"

	"gitlab.forceup.in/fil-data-factory/filscan-backend/modules/filscan/service/rewardparity"
)

// 端点集合的解析与「是否需要高度区间」是大额转账端点接入后新增/收紧的判断逻辑，
// 单测钉住三件事：
//
//  1. large_amount 是合法端点名（与 londobell 的 HTTP path 同名，便于日志对照）；
//  2. 只比 large_amount 时**不要求** -start/-end（它按 index/limit 取数，没有高度区间）；
//     但只要集合里还有统计端点，区间仍然必须给 —— 防止「-endpoints 全量」时静默按 [0,0) 跑；
//  3. 未知端点名报错并给出可选项（不能静默忽略，否则会得出「跑了但没比」的假结论）。
func TestParseEndpoints(t *testing.T) {
	cases := []struct {
		name    string
		in      string
		want    []string
		wantErr string
	}{
		{name: "只比大额转账", in: "large_amount", want: []string{"large_amount"}},
		{name: "混合端点", in: "wincount,large_amount", want: []string{"wincount", "large_amount"}},
		{name: "两侧空格被裁掉", in: " large_amount , wincount ", want: []string{"large_amount", "wincount"}},
		{name: "未知端点", in: "largeTransfer", wantErr: "未知端点"},
		{name: "大小写敏感（线上 path 也是这个大小写）", in: "Large_Amount", wantErr: "未知端点"},
		{name: "只有分隔符", in: " , ", wantErr: "未指定任何端点"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseEndpoints(c.in)
			if c.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("应报错含 %q，得到 err=%v got=%v", c.wantErr, err, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("不该报错: %s", err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("解析结果应为 %v，得到 %v", c.want, got)
			}
		})
	}
}

func TestNeedsEpochRange(t *testing.T) {
	if needsEpochRange([]string{rewardparity.EndpointLargeAmount}) {
		t.Error("只有大额转账时不该要求 -start/-end")
	}
	// 除大额转账外，AllEndpoints 里的每个端点都应按高度区间取数。
	for _, ep := range rewardparity.AllEndpoints() {
		if ep == rewardparity.EndpointLargeAmount {
			continue
		}
		if !needsEpochRange([]string{ep}) {
			t.Errorf("%s 按高度区间取数，必须要求 -start/-end", ep)
		}
	}
	if !needsEpochRange([]string{rewardparity.EndpointLargeAmount, "wincount"}) {
		t.Error("混合端点里只要有一个区间端点就必须要求区间")
	}
}

func TestContainsEndpoint(t *testing.T) {
	eps := []string{rewardparity.EndpointLargeAmount, "wincount"}
	if !containsEndpoint(eps, rewardparity.EndpointLargeAmount) {
		t.Error("应包含 large_amount")
	}
	if containsEndpoint(eps, "miner_blockreward") {
		t.Error("不该包含 miner_blockreward")
	}
}
