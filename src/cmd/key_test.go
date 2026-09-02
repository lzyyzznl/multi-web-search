package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidEngineName(t *testing.T) {
	valid := []string{"serper", "baidu", "brave", "tavily", "aliyun-iqs", "exa", "SERPER", "Baidu"}
	for _, name := range valid {
		if !validEngineName(name) {
			t.Errorf("validEngineName(%q) = false, want true", name)
		}
	}
	invalid := []string{"", "google", "bing", "serper "}
	for _, name := range invalid {
		if validEngineName(name) {
			t.Errorf("validEngineName(%q) = true, want false", name)
		}
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc123", "abc123"},
		{"has space", "has space"},
		{"it's", `it'\''s`},
		{"$HOME`id`;rm", "$HOME`id`;rm"},
		{"a'b'c", `a'\''b'\''c`},
	}
	for _, c := range cases {
		if got := shellQuote(c.in); got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestWriteProfileEnvDirMissing 覆盖父目录不存在（全新用户 HOME）场景：
// 写入应自动创建目录而非报错. 回归 bug: 原子写 os.CreateTemp 在目录缺失时失败.
func TestWriteProfileEnvDirMissing(t *testing.T) {
	home := filepath.Join(t.TempDir(), "deep", "nested", "home") // 目录不存在
	if err := writeProfileEnv(home, "SERPER_API_KEY", "k"); err != nil {
		t.Fatalf("目录不存在时写入失败: %v", err)
	}
	content, _ := os.ReadFile(filepath.Join(home, ".profile"))
	if !strings.Contains(string(content), "export SERPER_API_KEY='k'") {
		t.Errorf("内容不对: %q", string(content))
	}
}

func TestWriteProfileEnv(t *testing.T) {
	tmp := t.TempDir() // 纯函数直接传目录，不依赖真实 HOME

	// 文件不存在 → 创建并写入一行
	if err := writeProfileEnv(tmp, "SERPER_API_KEY", "key1"); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	profile := filepath.Join(tmp, ".profile")
	content, _ := os.ReadFile(profile)
	if !strings.Contains(string(content), "export SERPER_API_KEY='key1'") {
		t.Errorf("首次写入内容不对: %q", string(content))
	}

	// 再写另一个变量 → 追加，不动已有行
	if err := writeProfileEnv(tmp, "BAIDU_API_KEY", "key2"); err != nil {
		t.Fatalf("追加写入失败: %v", err)
	}
	content, _ = os.ReadFile(profile)
	if !strings.Contains(string(content), "export SERPER_API_KEY='key1'") ||
		!strings.Contains(string(content), "export BAIDU_API_KEY='key2'") {
		t.Errorf("追加后内容不对: %q", string(content))
	}

	// 覆盖同名变量 → 替换原行而非新增
	if err := writeProfileEnv(tmp, "SERPER_API_KEY", "newkey"); err != nil {
		t.Fatalf("覆盖写入失败: %v", err)
	}
	content, _ = os.ReadFile(profile)
	var serperLines []string
	for _, ln := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if strings.Contains(ln, "SERPER_API_KEY") {
			serperLines = append(serperLines, ln)
		}
	}
	if len(serperLines) != 1 || serperLines[0] != "export SERPER_API_KEY='newkey'" {
		t.Errorf("覆盖后 SERPER 行应为 1 条且为新值，got %v", serperLines)
	}

	// 特殊字符（含单引号）正确转义，无残留 tmp
	if err := writeProfileEnv(tmp, "EXA_API_KEY", "it's $HOME"); err != nil {
		t.Fatalf("特殊字符写入失败: %v", err)
	}
	content, _ = os.ReadFile(profile)
	if !strings.Contains(string(content), `export EXA_API_KEY='it'\''s $HOME'`) {
		t.Errorf("特殊字符转义不对: %q", string(content))
	}
	matches, _ := filepath.Glob(filepath.Join(tmp, ".profile-*.tmp"))
	if len(matches) != 0 {
		t.Errorf("残留临时文件: %v", matches)
	}
}
