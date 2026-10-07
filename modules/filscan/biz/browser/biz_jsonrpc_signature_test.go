/** @format */

package browser

import (
	"reflect"
	"testing"
)

// jsonrpc 注册对 biz 方法签名的硬约束。
//
// pkg/jsonrpc.Register 会把 *BrowserBiz 的**全部导出方法**登记成 JSON-RPC 方法，而底层
// go-jsonrpc 的 processFuncOut（github.com/ipfs-force-community/go-jsonrpc/util.go）只接受
// 0/1/2 个返回值，且 2 个时第 2 个必须是 error。违反 → **进程启动时 panic**
// （编译不报错、普通单测也照样绿；实测踩过：cali 换件时新二进制启动即 exit 2，
// supervisor 只报 "spawn error"，容易被误判成 exec 问题）。
//
// 本用例把该约束变成机检：往 biz 上加方法时，若有 3 个以上返回值会当场红。
// 反向自测（证明它不是永远绿的仪器）：临时加一个 3 返回值的方法再跑本用例，必须变红。
func TestBrowserBizMethodsJsonrpcSignature(t *testing.T) {
	errType := reflect.TypeOf((*error)(nil)).Elem()
	typ := reflect.TypeOf(&BrowserBiz{})
	if typ.NumMethod() == 0 {
		t.Fatal("没扫到任何方法，判据本身失效（探针必须能扫到方法）")
	}
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		n := m.Type.NumOut()
		if n > 2 {
			t.Errorf("BrowserBiz.%s 有 %d 个返回值：go-jsonrpc 注册会在启动时 panic（最多 2 个，第 2 个必须是 error）", m.Name, n)
			continue
		}
		if n == 2 && m.Type.Out(1) != errType {
			t.Errorf("BrowserBiz.%s 的第 2 个返回值必须是 error，实际是 %s", m.Name, m.Type.Out(1))
		}
	}
}
