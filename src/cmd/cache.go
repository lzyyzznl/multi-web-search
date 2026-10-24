package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cacheTTL 搜索结果的缓存有效期.
const cacheTTL = 15 * time.Minute

type cacheEntry struct {
	Query    string         `json:"query"`
	Primary  string         `json:"primary,omitempty"`
	Results  []SearchResult `json:"results"`
	Raw      int            `json:"raw"`
	CachedAt time.Time      `json:"cached_at"`
}

func cachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".cache", "multi-web-search")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return dir, nil
}

// cacheSignature 把影响检索结果的参数折叠成稳定签名，避免不同引擎集合/主引擎/条数
// 复用同一份缓存（否则 --engines aux 会命中全量缓存，主引擎优先级也会串味）.
func cacheSignature(enabled []string, primary string, num int) string {
	if num <= 0 {
		num = defaultNum
	}
	return strings.Join(enabled, ",") + "|" + primary + "|" + fmt.Sprint(num)
}

// cacheKey 对 query + 参数签名做 SHA-256 得到稳定缓存文件名.
func cacheKey(query, signature string) string {
	h := sha256.Sum256([]byte(query + "\x00" + signature))
	return hex.EncodeToString(h[:])
}

// loadCache 尝试读取缓存，若命中且未过期则返回结果.
func loadCache(query, signature string) (*SearchResponse, bool) {
	dir, err := cachePath()
	if err != nil {
		return nil, false
	}
	path := filepath.Join(dir, cacheKey(query, signature)+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var entry cacheEntry
	if json.Unmarshal(data, &entry) != nil {
		return nil, false
	}
	if time.Since(entry.CachedAt) > cacheTTL {
		return nil, false
	}
	return &SearchResponse{
		Query:   entry.Query,
		Primary: entry.Primary,
		Meta: SearchMeta{
			TotalRaw:    entry.Raw,
			TotalUnique: len(entry.Results),
		},
		Results: entry.Results,
	}, true
}

// saveCache 持久化搜索结果.
func saveCache(query, signature string, resp *SearchResponse) {
	dir, err := cachePath()
	if err != nil {
		return
	}
	entry := cacheEntry{
		Query:    query,
		Primary:  resp.Primary,
		Results:  resp.Results,
		Raw:      resp.Meta.TotalRaw,
		CachedAt: time.Now(),
	}
	data, _ := json.Marshal(entry)
	path := filepath.Join(dir, cacheKey(query, signature)+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, path) //nolint:errcheck
}
