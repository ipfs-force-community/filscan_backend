package contract

import (
	"path/filepath"
	"strings"
	"testing"
)

// SecureJoin 是「用户提供的文件名 -> 落盘路径」的唯一校验入口：
// VerifyContract / VerifyHardhatContract 的 4 个 file_name 调用点，以及
// CompileWithMetaData 里以用户 metadata.Sources 的 key 作相对路径的那处，都走它。
func TestSecureJoinRejectsEscapes(t *testing.T) {
	base := t.TempDir()
	payloads := []string{
		"../../etc/cron.d/x",
		"/etc/passwd",
		"..%2f..%2fx",
		"a/../../../b",
		"",
		"   ",
		"..",
		".",
		"./",
		"../",
		"foo/../../bar",
		`..\..\windows\system32\evil`,
		"a\x00b",
		"/etc/../etc/passwd",
		"..%5c..%5cx",
	}
	for _, p := range payloads {
		if got, err := SecureJoin(base, p); err == nil {
			t.Errorf("SecureJoin(%q) = %q, want error", p, got)
		}
	}
}

// 合法输入必须原样保留相对子目录（多文件合约的 import 解析依赖目录结构）。
func TestSecureJoinKeepsLegitSubPath(t *testing.T) {
	base := t.TempDir()
	absBase, err := filepath.Abs(base)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	cases := map[string]string{
		"contracts/Foo.sol":   filepath.Join("contracts", "Foo.sol"),
		"contracts/sub/A.sol": filepath.Join("contracts", "sub", "A.sol"),
		"hardhat.json":        "hardhat.json",
		"./A.sol":             "A.sol",
	}
	for in, wantRel := range cases {
		got, err := SecureJoin(base, in)
		if err != nil {
			t.Errorf("SecureJoin(%q) unexpected error: %v", in, err)
			continue
		}
		want := filepath.Join(absBase, wantRel)
		if got != want {
			t.Errorf("SecureJoin(%q) = %q, want %q", in, got, want)
		}
		if !strings.HasPrefix(got, absBase+string(filepath.Separator)) {
			t.Errorf("SecureJoin(%q) = %q, escaped base %q", in, got, absBase)
		}
	}
}

// "/tmp/dir 与 /tmp/dir2" 的文本前缀陷阱：兄弟目录不能被当成 base 之内。
func TestSecureJoinPrefixBoundary(t *testing.T) {
	base := t.TempDir()
	sibling := base + "2"
	absSibling, err := filepath.Abs(sibling)
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}
	got, err := SecureJoin(sibling, "hardhat.json")
	if err != nil {
		t.Fatalf("SecureJoin(%q) unexpected error: %v", sibling, err)
	}
	if !strings.HasPrefix(got, absSibling+string(filepath.Separator)) {
		t.Errorf("SecureJoin(%q) = %q, want under %q", sibling, got, absSibling)
	}
}
