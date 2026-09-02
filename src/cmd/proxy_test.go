package cmd

import (
	"os"
	"testing"
)

func TestParseRegistryProxyServer(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"127.0.0.1:10808", "127.0.0.1:10808"},                       // 简单格式
		{"http=127.0.0.1:7890;https=127.0.0.1:7891", "127.0.0.1:7891"}, // 多协议 → 优先 https
		{"http=127.0.0.1:7890;https=127.0.0.1:7891;ftp=127.0.0.1:21", "127.0.0.1:7891"},
		{"http=127.0.0.1:7890", "127.0.0.1:7890"}, // 只有 http 段
		{"https=127.0.0.1:7891", "127.0.0.1:7891"},
		{"", ""},
		{"   ", ""},
		{"=127.0.0.1:7890", ""}, // 无协议名
		{"http=127.0.0.1:7890;https=", "127.0.0.1:7890"}, // https 空 → 回退 http
	}
	for _, c := range cases {
		if got := parseRegistryProxyServer(c.in); got != c.want {
			t.Errorf("parseRegistryProxyServer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDetectLocalProxyHonorsEnv(t *testing.T) {
	os.Setenv("MULTI_WEB_SEARCH_NO_PROXY", "1")
	os.Setenv("HTTPS_PROXY", "")
	os.Setenv("https_proxy", "")
	if got := DetectLocalProxy(); got != "" {
		t.Errorf("NO_PROXY=1 应强制直连, got %q", got)
	}

	os.Setenv("MULTI_WEB_SEARCH_NO_PROXY", "")
	os.Setenv("HTTPS_PROXY", "http://127.0.0.1:9999")
	if got := DetectLocalProxy(); got != "http://127.0.0.1:9999" {
		t.Errorf("已有 HTTPS_PROXY 应沿用, got %q", got)
	}

	os.Setenv("HTTPS_PROXY", "")
	os.Setenv("https_proxy", "http://127.0.0.1:8888")
	if got := DetectLocalProxy(); got != "http://127.0.0.1:8888" {
		t.Errorf("已有 https_proxy 应沿用, got %q", got)
	}
	os.Setenv("https_proxy", "")
}

func TestApplyProxySetsEnv(t *testing.T) {
	os.Setenv("MULTI_WEB_SEARCH_NO_PROXY", "1")
	os.Setenv("HTTPS_PROXY", "")
	os.Setenv("HTTP_PROXY", "")
	os.Setenv("https_proxy", "")
	os.Setenv("http_proxy", "")
	ApplyProxy() // NO_PROXY=1 → 不注入
	if os.Getenv("HTTPS_PROXY") != "" {
		t.Errorf("NO_PROXY=1 时不应注入 HTTPS_PROXY, got %q", os.Getenv("HTTPS_PROXY"))
	}
	os.Setenv("MULTI_WEB_SEARCH_NO_PROXY", "")
}
