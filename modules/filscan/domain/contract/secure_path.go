package contract

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// SecureJoin 校验不可信的「用户提供的文件名」，返回保证落在 baseDir 内的绝对路径。
//
// 与「压成 basename（filepath.Base）」的做法不同，这里**保留合法的相对子目录**：
// contracts/Foo.sol 仍写到 <baseDir>/contracts/Foo.sol。这样做的原因是多文件合约的
// import 解析依赖磁盘目录结构，把路径压平会改变既有行为（可能让原本能过的校验失败）。
// 安全性不因此打折：任何越界形态都在下面被拒绝。
//
// 调用点（都来自请求 JSON，匿名可达）：
//   - VerifyContract / VerifyHardhatContract 的 SourceFile.FileName、MateDataFile.FileName、
//     HardhatBuildInfoFile.FileName（历史漏洞：直接拼接导致任意文件写）；
//   - CompileWithMetaData 的 JSONError 回退分支里、以用户提交的 metadata.Sources 的 key
//     作为落盘相对路径（同族漏洞，必须走同一套校验）。
//
// fail-closed 规则（任一违反即返回错误，绝不回退到原始输入）：
//   - 空 / 纯空白
//   - 含 NUL 字节（原始或解码后）
//   - 原始形式或 URL 解码后是绝对路径（/etc/passwd、..%2f..%2fx 等）
//   - 原始形式或 URL 解码后存在 ".." 路径元素（%2f/%5c/%2e 一律先解码再判）
//   - 归一化后为 "." / ".." / 仅分隔符 / 以 "../" 开头
//   - 兜底：clean 后的绝对路径必须仍被 baseDir 包含（前缀比较带分隔符，
//     避免 /tmp/dir 与 /tmp/dir2 这类「文本前缀相同」的误判）
func SecureJoin(baseDir, relName string) (string, error) {
	name := strings.TrimSpace(relName)
	if name == "" {
		return "", fmt.Errorf("invalid file name: empty")
	}
	if strings.IndexByte(name, 0) >= 0 {
		return "", fmt.Errorf("invalid file name: contains NUL byte")
	}
	// 防御性解码：上游代理/解码器可能已经把 %2f 翻成 "/"，所以编码态与解码态都要判。
	decoded, err := url.PathUnescape(name)
	if err != nil {
		return "", fmt.Errorf("invalid file name %q: %w", name, err)
	}
	if strings.IndexByte(decoded, 0) >= 0 {
		return "", fmt.Errorf("invalid file name: contains NUL byte after decoding")
	}
	if filepath.IsAbs(name) || filepath.IsAbs(decoded) {
		return "", fmt.Errorf("invalid file name %q: absolute paths are not allowed", name)
	}
	for _, cand := range []string{name, decoded} {
		for _, part := range strings.FieldsFunc(cand, func(r rune) bool { return r == '/' || r == '\\' }) {
			if part == ".." {
				return "", fmt.Errorf("invalid file name %q: path traversal is not allowed", name)
			}
		}
	}
	clean := filepath.Clean(decoded)
	sep := string(filepath.Separator)
	if clean == "." || clean == ".." || clean == sep || strings.HasPrefix(clean, ".."+sep) {
		return "", fmt.Errorf("invalid file name %q: invalid path", name)
	}
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", fmt.Errorf("invalid base dir %q: %w", baseDir, err)
	}
	joined := filepath.Clean(filepath.Join(absBase, clean))
	if joined == absBase {
		return "", fmt.Errorf("invalid file name %q: resolves to the base directory itself", name)
	}
	prefix := absBase
	if !strings.HasSuffix(prefix, sep) {
		prefix += sep
	}
	if !strings.HasPrefix(joined, prefix) {
		return "", fmt.Errorf("invalid file name %q: resolves outside %q", name, baseDir)
	}
	return joined, nil
}
