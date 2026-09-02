package cmd

import (
	"os"
	"testing"
)

// tempHome 为测试创建隔离 HOME，避免污染真实熔断状态.
// 注意：os.UserHomeDir() 在 Windows 读 USERPROFILE，Linux/macOS 读 HOME，
// 两者都要隔离，否则测试状态会持久化到真实用户目录.
func tempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldProfile := os.Getenv("USERPROFILE")
	os.Setenv("HOME", dir)
	os.Setenv("USERPROFILE", dir)
	t.Cleanup(func() {
		os.Setenv("HOME", oldHome)
		os.Setenv("USERPROFILE", oldProfile)
	})
	return dir
}

func TestCircuitBreakerClosedByDefault(t *testing.T) {
	tempHome(t)
	cb := NewCircuitBreaker("test-closed")
	if got := cb.Status(); got != "ok" {
		t.Errorf("expected ok status, got %q", got)
	}
	if cb.IsOpen() {
		t.Error("new circuit breaker should not be open")
	}
}

func TestCircuitBreakOnQuota(t *testing.T) {
	tempHome(t)
	cb := NewCircuitBreaker("test-quota")
	cb.Open("quota_exhausted", "429 rate limit")
	if !cb.IsOpen() {
		t.Error("expected circuit to be open after quota exhaustion")
	}
}

func TestConsecutiveFailuresTriggersOpen(t *testing.T) {
	tempHome(t)
	cb := NewCircuitBreaker("test-fail")
	// 连续失败 3 次（maxConsecutiveFailures）后应熔断.
	for i := 0; i < maxConsecutiveFailures; i++ {
		cb.RecordFailure("connection refused")
	}
	if !cb.IsOpen() {
		t.Errorf("expected circuit to open after %d consecutive failures", maxConsecutiveFailures)
	}
}

func TestSuccessResetsFailureCount(t *testing.T) {
	tempHome(t)
	cb := NewCircuitBreaker("test-reset")
	cb.RecordFailure("timeout")
	cb.RecordFailure("timeout")
	cb.RecordSuccess()
	// 成功后重置，再失败 2 次不应熔断.
	cb.RecordFailure("timeout")
	if cb.IsOpen() {
		t.Error("circuit should not be open: success reset the failure counter")
	}
}

func TestCachePersistenceAcrossInstances(t *testing.T) {
	tempHome(t)
	cb1 := NewCircuitBreaker("test-persist")
	cb1.Open("quota_exhausted", "429")

	// 新实例（模拟新进程）应能读到持久化状态.
	cb2 := NewCircuitBreaker("test-persist")
	if !cb2.IsOpen() {
		t.Error("persisted circuit state should be loaded by a new instance")
	}
}
