package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "管理搜索引擎 API Key（写入系统用户环境变量）",
}

var keyAddCmd = &cobra.Command{
	Use:   "add <engine> <api_key>",
	Short: "将 API Key 写入系统用户环境变量",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		engine := strings.ToLower(args[0])
		key := args[1]

		// 引擎名白名单校验（envVarMap 键全小写）
		if !validEngineName(engine) {
			var names []string
			for n := range envVarMap {
				names = append(names, n)
			}
			sort.Strings(names)
			return fmt.Errorf("未知引擎 %q，可用: %s", engine, strings.Join(names, ", "))
		}
		if key == "" {
			return fmt.Errorf("API Key 不能为空")
		}

		envVar := envVarMap[engine]

		// 平台持久化（key_windows.go / key_unix.go）
		if err := persistEnvVarPlatform(envVar, key); err != nil {
			return err
		}

		// 当前会话立即生效，后续 search/status 无需重开终端
		os.Setenv(envVar, key)

		fmt.Printf("✅ 已写入系统环境变量 %s\n", envVar)
		fmt.Println("   当前会话已生效；新进程/新终端将自动读取。")
		fmt.Printf("   运行 multi-web-search status 验证。\n")
		return nil
	},
}

// validEngineName 校验引擎名是否在 envVarMap 白名单内.
func validEngineName(name string) bool {
	_, ok := envVarMap[strings.ToLower(name)]
	return ok
}

func init() {
	keyCmd.AddCommand(keyAddCmd)
}
