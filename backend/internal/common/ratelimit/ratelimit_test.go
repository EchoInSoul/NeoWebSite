package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func newTestLimiter(now *time.Time) *Limiter {
	l := New()
	l.now = func() time.Time { return *now }
	l.lastSweep = *now
	return l
}

func TestAllowRejectsAtLimitAndRecoversAfterWindow(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	l := newTestLimiter(&now)

	for i := 0; i < 2; i++ {
		ok, _ := l.Allow("k", 2, time.Minute)
		if !ok {
			t.Fatalf("attempt %d = false, want true", i+1)
		}
	}

	ok, retry := l.Allow("k", 2, time.Minute)
	if ok {
		t.Fatal("third attempt = true, want false")
	}
	if retry != time.Minute {
		t.Fatalf("retry = %v, want %v", retry, time.Minute)
	}

	now = now.Add(time.Minute)
	ok, _ = l.Allow("k", 2, time.Minute)
	if !ok {
		t.Fatal("attempt after window = false, want true")
	}
}

func TestAllowKeysAreIndependent(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	l := newTestLimiter(&now)

	for _, key := range []string{"a", "b"} {
		if ok, _ := l.Allow(key, 1, time.Minute); !ok {
			t.Fatalf("first attempt on %q = false, want true", key)
		}
	}
	for _, key := range []string{"a", "b"} {
		if ok, _ := l.Allow(key, 1, time.Minute); ok {
			t.Fatalf("second attempt on %q = true, want false", key)
		}
	}
}

func TestBlockedCooldownAndRecovery(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	l := newTestLimiter(&now)

	l.RecordFailure("k", 2, time.Minute, 5*time.Minute)
	l.RecordFailure("k", 2, time.Minute, 5*time.Minute)

	blocked, retry := l.Blocked("k")
	if !blocked {
		t.Fatal("blocked = false, want true")
	}
	if retry != 5*time.Minute {
		t.Fatalf("retry = %v, want %v", retry, 5*time.Minute)
	}

	now = now.Add(5 * time.Minute)
	blocked, _ = l.Blocked("k")
	if blocked {
		t.Fatal("blocked after cooldown = true, want false")
	}

	l.RecordFailure("k", 2, time.Minute, 5*time.Minute)
	blocked, _ = l.Blocked("k")
	if blocked {
		t.Fatal("single failure after cooldown blocked, want false")
	}
}

func TestRecordFailureStaleWindowResets(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	l := newTestLimiter(&now)

	l.RecordFailure("k", 2, time.Minute, 5*time.Minute)
	now = now.Add(time.Minute + time.Second)
	l.RecordFailure("k", 2, time.Minute, 5*time.Minute)

	blocked, _ := l.Blocked("k")
	if blocked {
		t.Fatal("stale failure counted toward limit, want reset")
	}
}

func TestConcurrentFailuresAreCountedOnce(t *testing.T) {
	now := time.Now()
	l := newTestLimiter(&now)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.RecordFailure("k", 100, time.Hour, time.Hour)
		}()
	}
	wg.Wait()

	blocked, _ := l.Blocked("k")
	if !blocked {
		t.Fatal("100 concurrent failures did not reach the limit")
	}
}

func TestResetClearsFailureState(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	l := newTestLimiter(&now)

	l.RecordFailure("k", 2, time.Minute, time.Hour)
	l.Reset("k")
	l.RecordFailure("k", 2, time.Minute, time.Hour)

	blocked, _ := l.Blocked("k")
	if blocked {
		t.Fatal("failure before reset counted after reset")
	}
}

func TestSweepExpiresEntriesAndCapsMap(t *testing.T) {
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	l := newTestLimiter(&now)
	l.maxEntries = 2

	l.RecordFailure("a", 10, time.Hour, time.Hour)
	now = now.Add(time.Second)
	l.RecordFailure("b", 10, time.Hour, time.Hour)
	now = now.Add(time.Second)
	l.RecordFailure("c", 10, time.Hour, time.Hour)

	if len(l.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(l.entries))
	}
	if _, ok := l.entries["a"]; ok {
		t.Fatal("oldest key was not evicted")
	}

	now = time.Date(2026, 10, 20, 13, 0, 0, 0, time.UTC)
	l2 := newTestLimiter(&now)
	l2.sweepInterval = time.Second
	l2.entryTTL = time.Minute

	l2.RecordFailure("old", 10, time.Hour, time.Hour)
	now = now.Add(2 * time.Minute)
	l2.Allow("new", 1, time.Minute)

	if _, ok := l2.entries["old"]; ok {
		t.Fatal("expired key was not swept")
	}
	if _, ok := l2.entries["new"]; !ok {
		t.Fatal("new key missing after sweep")
	}
}
