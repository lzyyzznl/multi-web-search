package main

import "github.com/lzyyzznl/multi-web-search/cmd"

func main() {
	// 启动时检测本地代理并注入 HTTPS_PROXY/HTTP_PROXY：
	// 有可用代理（Windows 注册表 / 常见端口）走代理，无代理直连。
	cmd.ApplyProxy()
	cmd.Execute()
}
