package browser

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSecureContractFilePathRejectsTraversal asserts that every traversal /
// absolute / malformed payload is refused (fail-closed) by the shared helper.
func TestSecureContractFilePathRejectsTraversal(t *testing.T) {
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
		"..\\..\\windows\\system32\\evil",
		"a\x00b",
	}
	for _, payload := range payloads {
		got, err := secureContractFilePath(base, payload)
		if err == nil {
			t.Errorf("secureContractFilePath(%q) = %q, want error", payload, got)
		}
	}
}

// TestSecureContractFilePathAllowsLegitNames asserts that ordinary file names
// (including ones carrying a benign sub-directory) still resolve to a path
// under the allowed base directory.
func TestSecureContractFilePathAllowsLegitNames(t *testing.T) {
	base := t.TempDir()
	absBase, err := filepath.Abs(base)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", base, err)
	}
	prefix := absBase + string(filepath.Separator)

	cases := []struct{ in, wantRel string }{
		{"hardhat.json", "hardhat.json"},
		{"contracts/Foo.sol", filepath.Join("contracts", "Foo.sol")},
		{"Foo.sol", "Foo.sol"},
		{"metadata.json", "metadata.json"},
		{"hardhat_build_info.json", "hardhat_build_info.json"},
		{"contracts/sub/Bar.sol", filepath.Join("contracts", "sub", "Bar.sol")},
		{"./Foo.sol", "Foo.sol"},
	}
	for _, c := range cases {
		got, err := secureContractFilePath(base, c.in)
		if err != nil {
			t.Errorf("secureContractFilePath(%q) unexpected error: %v", c.in, err)
			continue
		}
		if !strings.HasPrefix(got, prefix) {
			t.Errorf("secureContractFilePath(%q) = %q, want path inside %q", c.in, got, prefix)
		}
		// 合法的相对子目录必须**原样保留**（多文件合约的 import 依赖目录结构）。
		want := filepath.Join(absBase, c.wantRel)
		if got != want {
			t.Errorf("secureContractFilePath(%q) = %q, want %q", c.in, got, want)
		}
		if filepath.Base(got) != filepath.Base(want) {
			t.Errorf("secureContractFilePath(%q) base = %q, want %q", c.in, filepath.Base(got), filepath.Base(want))
		}
	}
}

// TestSecureContractFilePathPrefixBoundary guards the "/tmp/dir vs /tmp/dir2"
// prefix-containment trap: a sibling directory sharing a textual prefix must
// never be treated as inside the base.
func TestSecureContractFilePathPrefixBoundary(t *testing.T) {
	base := t.TempDir()
	sibling := base + "2"
	got, err := secureContractFilePath(sibling, "hardhat.json")
	if err != nil {
		t.Fatalf("secureContractFilePath(%q) unexpected error: %v", sibling, err)
	}
	// The sibling base is its own directory; the result must sit under sibling,
	// not under base.
	absSibling, err := filepath.Abs(sibling)
	if err != nil {
		t.Fatalf("filepath.Abs(%q): %v", sibling, err)
	}
	if !strings.HasPrefix(got, absSibling+string(filepath.Separator)) {
		t.Errorf("secureContractFilePath(%q) = %q, want under %q", sibling, got, absSibling)
	}
	if strings.HasPrefix(got, filepath.Clean(base)+string(filepath.Separator)) {
		t.Errorf("secureContractFilePath(%q) = %q escaped into sibling prefix %q", sibling, got, base)
	}
}
