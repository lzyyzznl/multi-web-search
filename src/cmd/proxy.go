package cmd

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// proxyPorts 常见本地代理端口，Linux/macOS 下探测.
var proxyPorts = []string{"10808", "10809", "7890", "7897", "1080", "8081", "8888"}

// DetectLocalProxy 检测可用本地代理：
//   - 已设置 HTTPS_PROXY/https_proxy → 沿用，不覆盖
//   - Windows → 读注册表代理 (ProxyEnable/ProxyServer)，且端口确实在监听才用
//   - Linux/macOS → 探测常见本地代理端口
//
// 返回代理地址（如 http://127.0.0.1:10808）；无可用代理返回空串（直连）.
// MULTI_WEB_SEARCH_NO_PROXY=1 可强制跳过检测.
func DetectLocalProxy() string {
	if os.Getenv("MULTI_WEB_SEARCH_NO_PROXY") == "1" {
		return ""
	}
	if p := os.Getenv("HTTPS_PROXY"); p != "" {
		return p
	}
	if p := os.Getenv("https_proxy"); p != "" {
		return p
	}

	switch runtime.GOOS {
	case "windows":
		return detectWindowsProxy()
	default:
		return detectPortProxy()
	}
}

// ApplyProxy 程序启动时检测本地代理并注入 HTTPS_PROXY/HTTP_PROXY.
// Go 默认 Transport 的 ProxyFromEnvironment 自动读取这两个变量，
// 因此所有引擎统一走代理；无代理则保持直连.
func ApplyProxy() {
	p := DetectLocalProxy()
	if p == "" {
		return
	}
	os.Setenv("HTTPS_PROXY", p)
	os.Setenv("HTTP_PROXY", p)
}

// detectWindowsProxy 读取系统注册表代理并验证端口监听.
func detectWindowsProxy() string {
	const key = `HKCU\Software\Microsoft\Windows\CurrentVersion\Internet Settings`
	enable, err := regValue(key, "ProxyEnable")
	if err != nil || strings.TrimSpace(enable) != "0x1" {
		return ""
	}
	ps, err := regValue(key, "ProxyServer")
	if err != nil {
		return ""
	}
	ps = parseRegistryProxyServer(ps)
	if ps == "" {
		return ""
	}
	// 端口确实在监听（代理进程活着）才使用
	if !proxyPortAlive(ps, 300*time.Millisecond) {
		return ""
	}
	return "http://" + ps
}

// detectPortProxy 探测常见本地代理端口（Linux/macOS）.
func detectPortProxy() string {
	for _, port := range proxyPorts {
		addr := "127.0.0.1:" + port
		if proxyPortAlive(addr, 200*time.Millisecond) {
			return "http://" + addr
		}
	}
	return ""
}

// regValue 读取注册表字符串值（经 reg.exe，避免引入 x/sys 依赖）.
// 输出形如:
//
//	ProxyEnable    REG_DWORD    0x1
//	ProxyServer    REG_SZ    127.0.0.1:10808
func regValue(key, name string) (string, error) {
	out, err := exec.Command("reg", "query", key, "/v", name).Output()
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		for i := 1; i < len(f)-1; i++ {
			if f[i] == "REG_SZ" || f[i] == "REG_DWORD" {
				return strings.Join(f[i+1:], " "), nil
			}
		}
	}
	return "", fmt.Errorf("reg query %s/%s 未找到值", key, name)
}

// parseRegistryProxyServer 解析注册表 ProxyServer 值：
// 简单 "host:port" 原样返回；
// 多协议格式 "http=host:port;https=host:port" → 优先 https 段，其次 http 段.
func parseRegistryProxyServer(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "=") {
		return s
	}
	var httpVal string
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "https":
			if v := strings.TrimSpace(kv[1]); v != "" {
				return v
			}
		case "http":
			httpVal = strings.TrimSpace(kv[1])
		}
	}
	return httpVal
}

// proxyPortAlive 探测 host:port 是否可连接（代理进程活着）.
func proxyPortAlive(addr string, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
