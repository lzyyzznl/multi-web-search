package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewOCRRequestMultipart(t *testing.T) {
	req, err := newOCRRequest("https://example.test/v1/tools/ocr", "sk-test", "doc.pdf", []byte("PDFDATA"), "rich_layout_recognition")
	if err != nil {
		t.Fatalf("newOCRRequest: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer sk-test" {
		t.Errorf("Authorization = %q", got)
	}

	mr, err := req.MultipartReader()
	if err != nil {
		t.Fatalf("MultipartReader: %v", err)
	}
	var purpose, filename string
	var data []byte
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("NextPart: %v", err)
		}
		body, _ := io.ReadAll(part)
		switch part.FormName() {
		case "purpose":
			purpose = string(body)
		case "file":
			filename = part.FileName()
			data = body
		}
	}
	if purpose != "rich_layout_recognition" {
		t.Errorf("purpose = %q", purpose)
	}
	if filename != "doc.pdf" {
		t.Errorf("filename = %q", filename)
	}
	if string(data) != "PDFDATA" {
		t.Errorf("file data = %q", data)
	}
}

func TestParseOCRResponse(t *testing.T) {
	// 服务端识别期间发换行保活，JSON 前可能有前导空白.
	body := "\n\n" + `{"id":"ocr-1","object":"ocr.result","created":1,"purpose":"text_recognition",
		"page_count":2,"pages":[{"index":0,"text":"第一页","finish_reason":"stop"},
		{"index":1,"text":"第二页","finish_reason":"length"}],
		"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":15}}`
	res, err := parseOCRResponse([]byte(body))
	if err != nil {
		t.Fatalf("parseOCRResponse: %v", err)
	}
	if res.PageCount != 2 || len(res.Pages) != 2 || res.Pages[1].FinishReason != "length" {
		t.Errorf("解析结果异常: %+v", res)
	}
}

func TestParseOCRResponseError(t *testing.T) {
	_, err := parseOCRResponse([]byte(`{"error":{"message":"OCR did not finish in time","type":"gateway_timeout_error"}}`))
	if err == nil || !strings.Contains(err.Error(), "did not finish") {
		t.Fatalf("期望识别失败错误，得到 %v", err)
	}
}

func TestOCRText(t *testing.T) {
	one := &ocrResult{PageCount: 1, Pages: []ocrPage{{Index: 0, Text: "hello"}}}
	if got := ocrText(one); got != "hello" {
		t.Errorf("单页 = %q", got)
	}
	two := &ocrResult{PageCount: 2, Pages: []ocrPage{
		{Index: 0, Text: "a\n"}, {Index: 1, Text: "b"}}}
	want := "--- 第 1 页 ---\na\n--- 第 2 页 ---\nb"
	if got := ocrText(two); got != want {
		t.Errorf("多页 = %q, want %q", got, want)
	}
}

func TestShouldOCRByExtension(t *testing.T) {
	yes := []string{"https://x.test/a.PDF", "https://x.test/a.png?raw=1", "report.docx", "/tmp/slide.PPTX"}
	for _, s := range yes {
		if !shouldOCR(s) {
			t.Errorf("%q 应走 OCR", s)
		}
	}
	no := []string{"https://x.test/a.html", "https://x.test/article", "notes.txt"}
	for _, s := range no {
		// 非 http 的本地路径不嗅探，直接按扩展名判 false.
		if isOCRableName(s) {
			t.Errorf("%q 不应走 OCR", s)
		}
	}
	if isOCRableContentType("text/html; charset=utf-8") {
		t.Error("text/html 不应走 OCR")
	}
	if !isOCRableContentType("application/pdf") {
		t.Error("application/pdf 应走 OCR")
	}
}

func TestDownloadOCRSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("PNGDATA"))
	}))
	defer srv.Close()

	name, data, err := loadOCRSource(srv.URL + "/scan.png")
	if err != nil {
		t.Fatalf("loadOCRSource: %v", err)
	}
	if name != "scan.png" || string(data) != "PNGDATA" {
		t.Errorf("name=%q data=%q", name, data)
	}
}

func TestOCRSourceEndToEnd(t *testing.T) {
	var gotPurpose, gotName, gotAuth string
	var gotData []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/tools/ocr" {
			t.Errorf("请求路径 = %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Errorf("ParseMultipartForm: %v", err)
			return
		}
		gotPurpose = r.FormValue("purpose")
		file, hdr, err := r.FormFile("file")
		if err != nil {
			t.Errorf("FormFile: %v", err)
			return
		}
		defer file.Close()
		gotName = hdr.Filename
		gotData, _ = io.ReadAll(file)
		w.Header().Set("Content-Type", "application/json")
		// 与真实服务一致：结果 JSON 前可能有换行保活.
		_, _ = io.WriteString(w, "\n"+`{"id":"ocr-1","object":"ocr.result","purpose":"rich_layout_recognition","page_count":1,"pages":[{"index":0,"text":"识别结果","finish_reason":"stop"}],"usage":{"input_tokens":1,"output_tokens":1,"total_tokens":2}}`)
	}))
	defer srv.Close()

	oldBase := deepseekBaseURL
	deepseekBaseURL = srv.URL
	defer func() { deepseekBaseURL = oldBase }()
	t.Setenv("DEEPSEEK_API_KEY", "sk-test")

	file := filepath.Join(t.TempDir(), "scan.png")
	if err := os.WriteFile(file, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := ocrSource(file, "rich_layout_recognition")
	if err != nil {
		t.Fatalf("ocrSource: %v", err)
	}
	if gotAuth != "Bearer sk-test" || gotPurpose != "rich_layout_recognition" {
		t.Errorf("auth=%q purpose=%q", gotAuth, gotPurpose)
	}
	if gotName != "scan.png" || string(gotData) != "PNGDATA" {
		t.Errorf("name=%q data=%q", gotName, gotData)
	}
	if res.PageCount != 1 || ocrText(res) != "识别结果" {
		t.Errorf("结果异常: %+v", res)
	}
}
