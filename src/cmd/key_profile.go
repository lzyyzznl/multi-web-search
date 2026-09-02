package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// writeProfileEnv 把 export <NAME>='<value>' 行写入 <home>/.profile（用户级登录环境）。
// 已存在同名行则替换；单引号转义防注入；原子写（tmp+rename）防中途写坏。
// 纯逻辑、无平台依赖，便于跨平台单元测试。
func writeProfileEnv(home, name, value string) error {
	if home == "" {
		return fmt.Errorf("home 目录为空")
	}

	profile := filepath.Join(home, ".profile")
	line := "export " + name + "='" + shellQuote(value) + "'"

	// 读取现有内容（文件不存在视为空）
	content, err := os.ReadFile(profile)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("读取 %s 失败: %v", profile, err)
	}
	var lines []string
	if len(content) > 0 {
		lines = strings.Split(strings.TrimRight(string(content), "\n"), "\n")
	}

	// 查找并替换已有 export <NAME>= 行
	prefix := "export " + name + "="
	found := false
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), prefix) {
			lines[i] = line
			found = true
			break
		}
	}
	if !found {
		lines = append(lines, line)
	}

	// 原子写：写同目录 tmp 后 rename，避免中途断电留下半截文件
	dir := filepath.Dir(profile)
	tmp, err := os.CreateTemp(dir, ".profile-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %v", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // rename 成功后无害

	if _, err := tmp.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		tmp.Close()
		return fmt.Errorf("写入临时文件失败: %v", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("关闭临时文件失败: %v", err)
	}
	if err := os.Rename(tmpName, profile); err != nil {
		return fmt.Errorf("替换 %s 失败: %v", profile, err)
	}

	return nil
}

// shellQuote 用单引号包裹并转义其中的单引号（POSIX shell 规则），
// 使任意 API Key 字符（含 $、空格、引号）都能安全写入 export 行。
func shellQuote(s string) string {
	return strings.ReplaceAll(s, "'", `'\''`)
}
