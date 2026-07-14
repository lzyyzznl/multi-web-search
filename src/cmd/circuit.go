package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CircuitState 熔断状态.
type CircuitState string

const (
	StateClosed CircuitState = "closed"
	StateOpen   CircuitState = "open"
)

// circuitData 持久化到磁盘的熔断记录.
type circuitData struct {
	State     CircuitState `json:"state"`
	OpenAt    time.Time    `json:"open_at"`
	OpenUntil time.Time    `json:"open_until"`
	Reason    string       `json:"reason"`
	LastError string       `json:"last_error,omitempty"`
}

// CircuitBreaker 每个引擎独立的熔断器.
type CircuitBreaker struct {
	mu       sync.Mutex
	name     string
	data     circuitData
	dirty    bool // 需要持久化
}

const circuitFile = "circuit.json"

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

func NewCircuitBreaker(name string) *CircuitBreaker {
	cb := &CircuitBreaker{name: name, data: circuitData{State: StateClosed}}
	cb.load()
	return cb
}

// load 从磁盘读取持久化状态.
func (cb *CircuitBreaker) load() {
	path, err := circuitPath()
	if err != nil {
		return
	}
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

// save 持久化熔断状态.
func (cb *CircuitBreaker) save() {
	path, err := circuitPath()
	if err != nil {
		return
	}
	store := make(map[string]circuitData)
	if existing, err := os.ReadFile(path); err == nil {
		json.Unmarshal(existing, &store) //nolint:errcheck
	}
	store[cb.name] = cb.data
	data, _ := json.MarshalIndent(store, "", "  ")
	os.WriteFile(path, data, 0644) //nolint:errcheck
}

// IsOpen 检查该引擎是否处于熔断状态.
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.data.State == StateOpen && time.Now().After(cb.data.OpenUntil) {
		cb.data.State = StateClosed
		cb.save()
		return false
	}
	return cb.data.State == StateOpen
}

// Open 熔断该引擎 24 小时.
func (cb *CircuitBreaker) Open(reason, lastErr string) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	now := time.Now()
	cb.data = circuitData{
		State:     StateOpen,
		OpenAt:    now,
		OpenUntil: now.Add(24 * time.Hour),
		Reason:    reason,
		LastError: lastErr,
	}
	cb.save()
}

// Status 返回当前熔断状态文本.
func (cb *CircuitBreaker) Status() string {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if cb.data.State == StateClosed {
		return "ok"
	}
	remaining := time.Until(cb.data.OpenUntil).Truncate(time.Second)
	return fmt.Sprintf("熔断中 (剩余 %v) — %s", remaining, cb.data.Reason)
}
