//go:build windows

package cmd

import (
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"
)

// persistEnvVarPlatform Windows 实现：写 HKCU\Environment 注册表（用户级，无需管理员），
// 随后广播 WM_SETTINGCHANGE 让 explorer 与新建进程立即感知新环境变量。
func persistEnvVarPlatform(name, value string) error {
	// reg add 通过 argv 直传，不经 shell，无注入风险。
	cmd := exec.Command("reg", "add", `HKCU\Environment`,
		"/v", name, "/t", "REG_SZ", "/d", value, "/f")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("写入注册表失败: %v (%s)", err, string(out))
	}

	broadcastEnvChange()
	return nil
}

// broadcastEnvChange 广播 WM_SETTINGCHANGE，通知系统环境变量已变更。
// 失败不阻断主流程（部分精简环境无 explorer），忽略错误即可。
func broadcastEnvChange() {
	user32 := syscall.NewLazyDLL("user32.dll")
	procSendMessageTimeoutW := user32.NewProc("SendMessageTimeoutW")

	const (
		hwndBroadcast   = 0xffff
		wmSettingChange = 0x001A
		sMTOAbortIfHung = 0x0002
	)

	// lParam 指向 "Environment" 字符串，告知变更的是环境变量区。
	envName, _ := syscall.UTF16PtrFromString("Environment")

	procSendMessageTimeoutW.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(envName)),
		uintptr(sMTOAbortIfHung),
		uintptr(5000),
		0,
	)
}
