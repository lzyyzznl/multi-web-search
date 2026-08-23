package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// cacheTTL 搜索结果的缓存有效期.
const cacheTTL = 15 * time.Minute

type cacheEntry struct {
	Query     string        `json:"query"`
	Results   []SearchResult `json:"results"`
	Raw       int           `json:"raw"`
	CachedAt  time.Time     `json:"cached_at"`
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

// cacheKey 对 query 做 SHA-256 得到稳定缓存文件名.
func cacheKey(query string) string {
	h := sha256.Sum256([]byte(query))
	return hex.EncodeToString(h[:])
}

// loadCache 尝试读取缓存，若命中且未过期则返回结果.
func loadCache(query string) (*SearchResponse, bool) {
	dir, err := cachePath()
	if err != nil {
		return nil, false
	}
	path := filepath.Join(dir, cacheKey(query)+".json")
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
		Query: entry.Query,
		Meta: SearchMeta{
			TotalRaw:    entry.Raw,
			TotalUnique: len(entry.Results),
		},
		Results: entry.Results,
	}, true
}

// saveCache 持久化搜索结果.
func saveCache(query string, resp *SearchResponse) {
	dir, err := cachePath()
	if err != nil {
		return
	}
	entry := cacheEntry{
		Query:    query,
		Results:  resp.Results,
		Raw:      resp.Meta.TotalRaw,
		CachedAt: time.Now(),
	}
	data, _ := json.Marshal(entry)
	path := filepath.Join(dir, cacheKey(query)+".json")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, path) //nolint:errcheck
}
