package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// circuitData 持久化到磁盘的熔断记录.
type circuitData struct {
	State     string    `json:"state"`
	OpenAt    time.Time `json:"open_at"`
	OpenUntil time.Time `json:"open_until"`
	Reason    string    `json:"reason"`
	LastError string    `json:"last_error,omitempty"`
	// ConsecutiveFailures 连续失败次数，用于瞬时错误熔断阈值.
	ConsecutiveFailures int `json:"consecutive_failures,omitempty"`
}

// CircuitBreaker 每个引擎独立的熔断器.
type CircuitBreaker struct {
	mu   sync.Mutex
	name string
	data circuitData
}

const circuitFile = "circuit.json"

// maxConsecutiveFailures 连续失败达到该次数后熔断（网络/5xx 类瞬时错误）.
const maxConsecutiveFailures = 3

func NewCircuitBreaker(name string) *CircuitBreaker {
	cb := &CircuitBreaker{name: name, data: circuitData{State: "closed"}}
	cb.load()
	return cb
}

// circuitPath 返回熔断状态文件路径.
func circuitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "multi-web-search")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, circuitFile), nil
}

// fileMu 保护 circuit.json 的并发读写.
//
// save 采用 读-改-写 而非原子替换：多个熔断器实例（不同引擎）并发熔断时，
// 若不互斥，后写者会覆盖先写者导致状态丢失。这里用进程级互斥锁保证串行。
var fileMu sync.Mutex

// load 从磁盘读取持久化状态.
func (cb *CircuitBreaker) load() {
	path, err := circuitPath()
	if err != nil {
		return
	}
	fileMu.Lock()
	defer fileMu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var store map[string]circuitData
	if json.Unmarshal(data, &store) != nil {
		return
	}
	if d, ok := store[cb.name]; ok {
		cb.data = d
	}
}

// save 持久化熔断状态（进程级互斥 + 原子写）.
func (cb *CircuitBreaker) save() {
	path, err := circuitPath()
	if err != nil {
		return
	}
	fileMu.Lock()
	defer fileMu.Unlock()

	store := make(map[string]circuitData)
	if existing, err := os.ReadFile(path); err == nil {
		json.Unmarshal(existing, &store) //nolint:errcheck
	}
	store[cb.name] = cb.data
	data, _ := json.MarshalIndent(store, "", "  ")

	// 原子写：先写临时文件再 rename，避免崩溃留下损坏 JSON.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return
	}
	os.Rename(tmp, path) //nolint:errcheck
}

// IsOpen 检查该引擎是否处于熔断状态.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.data.State == "open" && time.Now().After(cb.data.OpenUntil) {
		cb.data.State = "closed"
		cb.save()
		return false
	}
	return cb.data.State == "open"
}

// Open 熔断该引擎 24 小时.
func (cb *CircuitBreaker) Open(reason, lastErr string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	now := time.Now()
	cb.data = circuitData{
		State:     "open",
		OpenAt:    now,
		OpenUntil: now.Add(24 * time.Hour),
		Reason:    reason,
		LastError: lastErr,
	}
	cb.save()
}

// RecordFailure 记录一次瞬时错误失败，连续达到阈值后熔断 24h.
func (cb *CircuitBreaker) RecordFailure(lastErr string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.data.ConsecutiveFailures++
	cb.data.LastError = lastErr
	if cb.data.ConsecutiveFailures >= maxConsecutiveFailures {
		now := time.Now()
		cb.data.State = "open"
		cb.data.OpenAt = now
		cb.data.OpenUntil = now.Add(24 * time.Hour)
		cb.data.Reason = "consecutive_failures"
		cb.data.ConsecutiveFailures = 0
	}
	cb.save()
}

// RecordSuccess 记录一次成功，重置连续失败计数.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.data.ConsecutiveFailures != 0 {
		cb.data.ConsecutiveFailures = 0
		cb.save()
	}
}

// Status 返回当前熔断状态文本.
func (cb *CircuitBreaker) Status() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.data.State == "closed" {
		return "ok"
	}
	remaining := time.Until(cb.data.OpenUntil).Truncate(time.Second)
	return fmt.Sprintf("熔断中 (剩余 %v) — %s", remaining, cb.data.Reason)
}
