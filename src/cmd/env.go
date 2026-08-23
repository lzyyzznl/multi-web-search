package cmd

import "os"

// envVarMap 引擎名称 → 环境变量名.
var envVarMap = map[string]string{
	"serper":    "SERPER_API_KEY",
	"baidu":     "BAIDU_API_KEY",
	"brave":     "BRAVE_API_KEY",
	"tavily":    "TAVILY_API_KEY",
	"aliyun-iqs": "ALIYUN_IQS_API_KEY",
	"exa":       "EXA_API_KEY",
}

// DetectEnabledEngines 遍历环境变量，返回有 API Key 的引擎列表.
func DetectEnabledEngines() []string {
	var enabled []string
	for name, env := range envVarMap {
		if os.Getenv(env) != "" {
			enabled = append(enabled, name)
		}
	}
	return enabled
}
