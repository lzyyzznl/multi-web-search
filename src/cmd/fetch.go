package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"
)

// fetchCmd 通过 DeepSeek Fetch 接口抓取单个网页，以 Markdown 返回正文.
//
// 接口文档：https://api-docs.deepseek.com/zh-cn/guides/search
var fetchCmd = &cobra.Command{
	Use:   "fetch <url>",
	Short: "抓取网页正文为 Markdown（DeepSeek Fetch API，需 DEEPSEEK_API_KEY）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		rawURL := args[0]

		apiKey := os.Getenv(envVarMap["deepseek"])
		if apiKey == "" {
			return fmt.Errorf("deepseek: API Key 未配置 (%s)", envVarMap["deepseek"])
		}

		page, err := deepseekFetch(apiKey, rawURL)
		if err != nil {
			return err
		}

		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(struct {
				URL     string `json:"url"`
				Title   string `json:"title"`
				Content string `json:"content"`
			}{URL: rawURL, Title: page.Title, Content: page.Content})
		}

		if page.Title != "" {
			fmt.Printf("# %s\n\n", page.Title)
		}
		fmt.Println(page.Content)
		return nil
	},
}

// fetchedPage DeepSeek Fetch 接口响应.
type fetchedPage struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// deepseekFetch 调用 https://api.deepseek.com/v1/tools/fetch 抓取网页 Markdown 正文.
func deepseekFetch(apiKey, rawURL string) (*fetchedPage, error) {
	payload, _ := json.Marshal(map[string]string{"url": rawURL})
	req, err := http.NewRequest("POST",
		"https://api.deepseek.com/v1/tools/fetch", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("deepseek fetch: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("deepseek fetch: 429 rate limit")
	}
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("deepseek fetch: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var page fetchedPage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("deepseek fetch: decode: %w", err)
	}
	return &page, nil
}
