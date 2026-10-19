package ratelimit

import (
	"sync"
	"time"
)

const (
	defaultMaxEntries    = 100000
	defaultSweepInterval = time.Minute
	defaultEntryTTL      = 2 * time.Hour
)

type entry struct {
	attempts     int
	windowStart  time.Time
	failures     int
	lastFailure  time.Time
	blockedUntil time.Time
	lastSeen     time.Time
}

type Limiter struct {
	mu            sync.Mutex
	entries       map[string]*entry
	now           func() time.Time
	maxEntries    int
	sweepInterval time.Duration
	entryTTL      time.Duration
	lastSweep     time.Time
}

func New() *Limiter {
	l := &Limiter{
		entries:       make(map[string]*entry),
		now:           time.Now,
		maxEntries:    defaultMaxEntries,
		sweepInterval: defaultSweepInterval,
		entryTTL:      defaultEntryTTL,
	}
	l.lastSweep = l.now()
	return l
}

func (l *Limiter) Allow(key string, limit int, window time.Duration) (bool, time.Duration) {
	if limit <= 0 {
		return true, 0
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)
	e := l.entryLocked(key, now)
	l.evictLocked()

	if e.windowStart.IsZero() || !now.Before(e.windowStart.Add(window)) {
		e.attempts = 0
		e.windowStart = now
	}
	if e.attempts >= limit {
		retry := e.windowStart.Add(window).Sub(now)
		if retry < 0 {
			retry = 0
		}
		return false, retry
	}

	e.attempts++
	return true, 0
}

func (l *Limiter) Blocked(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)
	e, ok := l.entries[key]
	if !ok {
		return false, 0
	}
	e.lastSeen = now

	if e.blockedUntil.After(now) {
		return true, e.blockedUntil.Sub(now)
	}
	if !e.blockedUntil.IsZero() {
		e.failures = 0
		e.lastFailure = time.Time{}
		e.blockedUntil = time.Time{}
	}
	return false, 0
}

func (l *Limiter) RecordFailure(key string, limit int, window, cooldown time.Duration) {
	if limit <= 0 {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)
	e := l.entryLocked(key, now)
	l.evictLocked()

	if e.blockedUntil.After(now) {
		return
	}
	if !e.blockedUntil.IsZero() {
		e.failures = 0
		e.lastFailure = time.Time{}
		e.blockedUntil = time.Time{}
	}
	if e.failures > 0 && now.Sub(e.lastFailure) > window {
		e.failures = 0
	}

	e.failures++
	e.lastFailure = now
	if e.failures >= limit {
		e.blockedUntil = now.Add(cooldown)
	}
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func (l *Limiter) entryLocked(key string, now time.Time) *entry {
	e, ok := l.entries[key]
	if !ok {
		e = &entry{}
		l.entries[key] = e
	}
	e.lastSeen = now
	return e
}

func (l *Limiter) sweepLocked(now time.Time) {
	if !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < l.sweepInterval {
		l.evictLocked()
		return
	}

	for key, e := range l.entries {
		if now.Sub(e.lastSeen) > l.entryTTL {
			delete(l.entries, key)
		}
	}
	l.lastSweep = now
	l.evictLocked()
}

func (l *Limiter) evictLocked() {
	if l.maxEntries <= 0 {
		return
	}

	for len(l.entries) > l.maxEntries {
		var (
			oldestKey string
			oldest    time.Time
			found     bool
		)
		for key, e := range l.entries {
			if !found || e.lastSeen.Before(oldest) {
				oldestKey = key
				oldest = e.lastSeen
				found = true
			}
		}
		if !found {
			return
		}
		delete(l.entries, oldestKey)
	}
}
