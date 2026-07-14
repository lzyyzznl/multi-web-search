package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "multi-web-search",
	Short: "多引擎搜索聚合 — Serper/Baidu/Brave/Tavily/阿里云IQS",
}

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "并行搜索所有可用引擎并返回聚合结果",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		raw, _ := cmd.Flags().GetBool("raw")
		query := args[0]

		resp, err := doSearch(query)
		if err != nil {
			return err
		}

		switch {
		case raw || jsonOut:
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(resp)
		default:
			printPretty(resp)
			return nil
		}
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "查看各搜索引擎配额/熔断状态",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("🔍 搜索引擎状态\n")
		for name, env := range envVarMap {
			key := os.Getenv(env)
			if key == "" {
				fmt.Printf("  ❌ %-12s 未配置 (%s)\n", name, env)
				continue
			}
			cb := NewCircuitBreaker(name)
			status := cb.Status()
			if status == "ok" {
				fmt.Printf("  ✅ %-12s 已配置，状态正常\n", name)
			} else {
				fmt.Printf("  ⚠️  %-12s %s\n", name, status)
			}
		}
		return nil
	},
}

func printPretty(resp *SearchResponse) {
	fmt.Printf("\n搜索 \"%s\" — 来自 %d 个引擎\n\n", resp.Query, len(resp.EngineStatus))

	for name, s := range resp.EngineStatus {
		switch s.Status {
		case "ok":
			fmt.Printf("  🔵 %-12s (%d 条, %dms)\n", name, s.Results, s.LatencyMs)
		case "circuit_open":
			fmt.Printf("  ⚪ %-12s 跳过 (%s)\n", name, s.Error)
		case "error":
			fmt.Printf("  🔴 %-12s 失败 (%s)\n", name, s.Error)
		}
	}
	fmt.Println()

	for i, r := range resp.Results {
		const maxSnippet = 120
		snippet := r.Snippet
		if len(snippet) > maxSnippet {
			snippet = snippet[:maxSnippet] + "..."
		}
		fmt.Printf("  %d. %s\n", i+1, r.Title)
		fmt.Printf("     %s\n", r.URL)
		fmt.Printf("     %s  [%.2f | %s]\n", snippet, r.Score, r.Source)
		fmt.Println()
	}

	fmt.Printf("  📊 总计 %d 条原始结果，去重后 %d 条唯一结果 (耗时 %dms)\n",
		resp.Meta.TotalRaw, resp.Meta.TotalUnique, resp.Meta.DurationMs)
}

func Execute() {
	searchCmd.Flags().Bool("json", false, "JSON 格式输出")
	searchCmd.Flags().Bool("raw", false, "jq 友好 JSON")

	rootCmd.Flags().BoolP("version", "v", false, "print version and exit")
	rootCmd.AddCommand(searchCmd, statusCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Version = "1.0.0"
}
