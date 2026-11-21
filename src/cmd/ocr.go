package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	ocrMaxBytes = 100 << 20
	ocrTimeout  = 190 * time.Second
)

// deepseekBaseURL 单独成变量，便于测试注入本地 mock.
var deepseekBaseURL = "https://api.deepseek.com"

// ocrCmd 识别 PDF / Word / PowerPoint / 图片中的文字，按页输出.
var ocrCmd = &cobra.Command{
	Use:   "ocr <file-or-url>",
	Short: "识别 PDF/Word/PPT/图片文字（DeepSeek OCR，需 DEEPSEEK_API_KEY）",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		jsonOut, _ := cmd.Flags().GetBool("json")
		purpose, _ := cmd.Flags().GetString("purpose")
		if purpose != "text_recognition" && purpose != "rich_layout_recognition" {
			return fmt.Errorf("ocr: --purpose 只能是 text_recognition 或 rich_layout_recognition")
		}

		res, err := ocrSource(args[0], purpose)
		if err != nil {
			return err
		}
		if jsonOut {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}
		fmt.Println(ocrText(res))
		return nil
	},
}

// ocrPage 单页识别结果.
type ocrPage struct {
	Index        int    `json:"index"`
	Text         string `json:"text"`
	FinishReason string `json:"finish_reason"`
}

// ocrResult OCR 接口成功响应.
type ocrResult struct {
	ID        string    `json:"id"`
	Object    string    `json:"object"`
	Created   int64     `json:"created"`
	Purpose   string    `json:"purpose"`
	PageCount int       `json:"page_count"`
	Pages     []ocrPage `json:"pages"`
	Usage     struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// ocrResponse 响应体：识别失败时 HTTP 可能已是 200，错误在 body 的 error 字段.
type ocrResponse struct {
	ocrResult
	Error *struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

// ocrText 拼接所有页文本；多页时加页码分隔.
func ocrText(res *ocrResult) string {
	var b strings.Builder
	for _, p := range res.Pages {
		if len(res.Pages) > 1 {
			fmt.Fprintf(&b, "--- 第 %d 页 ---\n", p.Index+1)
		}
		b.WriteString(p.Text)
		if !strings.HasSuffix(p.Text, "\n") {
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// ocrSource 读取本地文件或下载 http(s) 资源，调用 DeepSeek OCR.
func ocrSource(source, purpose string) (*ocrResult, error) {
	apiKey := os.Getenv(envVarMap["deepseek"])
	if apiKey == "" {
		return nil, fmt.Errorf("deepseek: API Key 未配置 (%s)", envVarMap["deepseek"])
	}
	name, data, err := loadOCRSource(source)
	if err != nil {
		return nil, err
	}
	return deepseekOCR(apiKey, name, data, purpose)
}

// loadOCRSource 返回带文件名的原始字节：http(s) 下载，其余按本地路径读取.
func loadOCRSource(source string) (string, []byte, error) {
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		return downloadOCRSource(source)
	}
	info, err := os.Stat(source)
	if err != nil {
		return "", nil, fmt.Errorf("ocr: %w", err)
	}
	if info.Size() > ocrMaxBytes {
		return "", nil, fmt.Errorf("ocr: 文件过大 (%d 字节，上限 100 MiB)", info.Size())
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return "", nil, fmt.Errorf("ocr: %w", err)
	}
	return path.Base(source), data, nil
}

// downloadOCRSource 下载远程文件，文件名取 URL 末段（缺扩展名时由接口按内容判断格式）.
func downloadOCRSource(rawURL string) (string, []byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("ocr: %w", err)
	}
	req.Header.Set("User-Agent", "multi-web-search")
	client := &http.Client{Timeout: ocrTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("ocr: 下载失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", nil, fmt.Errorf("ocr: 下载 HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, ocrMaxBytes+1))
	if err != nil {
		return "", nil, fmt.Errorf("ocr: 下载失败: %w", err)
	}
	if len(data) > ocrMaxBytes {
		return "", nil, fmt.Errorf("ocr: 文件过大（上限 100 MiB）")
	}
	name := path.Base(resp.Request.URL.Path)
	if name == "" || name == "." || name == "/" {
		name = "file"
	}
	return name, data, nil
}

// newOCRRequest 组装 multipart 请求；endpoint 单独传参以便测试.
func newOCRRequest(endpoint, apiKey, filename string, data []byte, purpose string) (*http.Request, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if err := w.WriteField("purpose", purpose); err != nil {
		return nil, fmt.Errorf("deepseek ocr: %w", err)
	}
	part, err := w.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("deepseek ocr: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("deepseek ocr: %w", err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("deepseek ocr: %w", err)
	}

	req, err := http.NewRequest("POST", endpoint, &buf)
	if err != nil {
		return nil, fmt.Errorf("deepseek ocr: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req, nil
}

// deepseekOCR 以 multipart/form-data 调用 POST /v1/tools/ocr.
func deepseekOCR(apiKey, filename string, data []byte, purpose string) (*ocrResult, error) {
	req, err := newOCRRequest(deepseekBaseURL+"/v1/tools/ocr", apiKey, filename, data, purpose)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: ocrTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deepseek ocr: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("deepseek ocr: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("deepseek ocr: HTTP %d: %s", resp.StatusCode, truncateRunes(strings.TrimSpace(string(body)), 300))
	}
	return parseOCRResponse(body)
}

// parseOCRResponse 解析 200 响应：识别失败时 body 是 error 对象，不是识别结果.
func parseOCRResponse(body []byte) (*ocrResult, error) {
	var out ocrResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("deepseek ocr: decode: %w", err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("deepseek ocr: %s", out.Error.Message)
	}
	return &out.ocrResult, nil
}

// ocrExtensions OCR 接口支持的文档/图片扩展名.
var ocrExtensions = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".ppt": true, ".pptx": true,
	".png": true, ".jpg": true, ".jpeg": true, ".webp": true,
	".gif": true, ".bmp": true, ".tif": true, ".tiff": true,
}

// isOCRableName 按 URL/文件名扩展名判断是否交给 OCR.
func isOCRableName(name string) bool {
	return ocrExtensions[strings.ToLower(path.Ext(name))]
}

// isOCRableContentType 判断 Content-Type 是否为 OCR 支持的文档/图片.
func isOCRableContentType(ct string) bool {
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	switch strings.ToLower(strings.TrimSpace(ct)) {
	case "application/pdf",
		"application/msword", "application/vnd.openxmlformats.wordprocessingml.document",
		"application/vnd.ms-powerpoint", "application/vnd.openxmlformats.presentationml.presentation",
		"image/png", "image/jpeg", "image/webp", "image/gif", "image/bmp", "image/tiff":
		return true
	}
	return false
}

// shouldOCR 判断 fetch 目标是否该走 OCR：先看扩展名，再嗅探 Content-Type（失败即按网页处理）.
func shouldOCR(rawURL string) bool {
	if u, err := url.Parse(rawURL); err == nil && isOCRableName(u.Path) {
		return true
	}
	return isOCRableContentType(sniffContentType(rawURL))
}

// sniffContentType 只取响应头：Range 请求避免下载正文，失败返回空串.
func sniffContentType(rawURL string) string {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Range", "bytes=0-0")
	req.Header.Set("User-Agent", "multi-web-search")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	return resp.Header.Get("Content-Type")
}
