package londobell

import "fmt"

// 聚合器/适配器 HTTP 响应体中的业务码（code 字段）。
//
// 说明：这两个上游服务用 HTTP 200 + 响应体 code 表达业务结果，
// 因此「业务错误」在调用侧表现为普通 error，而不是 HTTP 传输错误。
const (
	CodeOK       = "0" // 成功
	CodeBusiness = "1" // 业务错误：服务端处理该请求时失败（例如该高度的数据无法解析/序列化）
	CodeNotFound = "2" // 数据不存在（客户端实现会转成 impl.ErrNotFound 哨兵错误）
)

// BusinessError 业务错误：HTTP 请求成功，但响应体 code 非 0（且非 2）。
//
// 与传输级错误（dial/EOF/超时/连接被拒 等）区分：业务错误意味着「服务端明确处理了本次请求并失败」；
// 对于按高度取数的聚合器接口（如 /aggregators/traces），重复请求同一高度不会自愈 —— 属于数据级错误，
// 由同步器决定是否按阈值跳过该高度（见 modules/syncer 的错误分类判据）。
type BusinessError struct {
	Code    string // 业务码
	Message string // 服务端返回的 msg
	URL     string // 请求地址，便于定位是哪个接口
}

func (e *BusinessError) Error() string {
	if e == nil {
		return ""
	}
	// 与改造前的字符串错误文本保持一致（改造前 err = fmt.Errorf(msg)），
	// 避免影响既有日志检索与人工判据。
	return e.Message
}

// DecodeError 响应体解码失败：服务端返回了数据，但该数据无法被解析成目标结构。
// 属于数据级错误：同一高度重试不会自愈（例如链上数据本身的编码非法）。
type DecodeError struct {
	Err error  // 原始解码错误（json.SyntaxError / json.UnmarshalTypeError 等）
	URL string // 请求地址，便于定位是哪个接口
}

func (e *DecodeError) Error() string {
	if e == nil {
		return ""
	}
	// 与改造前的文本保持一致（改造前 err = fmt.Errorf("unmarshal error:%s", err)）。
	return fmt.Sprintf("unmarshal error:%s", e.Err)
}

func (e *DecodeError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
