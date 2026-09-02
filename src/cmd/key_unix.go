//go:build !windows

package cmd

import (
	"fmt"
	"os"
)

// persistEnvVarPlatform Unix 实现：把 export 行写入 ~/.profile（用户级登录环境）。
// 纯逻辑见 key_profile.go（writeProfileEnv），此处只取用户主目录并分发。
func persistEnvVarPlatform(name, value string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("无法获取用户主目录: %v", err)
	}
	return writeProfileEnv(home, name, value)
}
