package ratelimit

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

type WindowPolicy struct {
	Limit  int
	Window time.Duration
}

type FailurePolicy struct {
	Limit    int
	Window   time.Duration
	Cooldown time.Duration
}

func AllowWindow(l *Limiter, policy WindowPolicy, key string) (bool, time.Duration) {
	if policy.Limit <= 0 {
		return true, 0
	}
	return l.Allow(key, policy.Limit, policy.Window)
}

func CheckFailure(l *Limiter, policy FailurePolicy, key string) (bool, time.Duration) {
	if policy.Limit <= 0 {
		return false, 0
	}
	return l.Blocked(key)
}

func RecordFailure(l *Limiter, policy FailurePolicy, key string) {
	if policy.Limit <= 0 {
		return
	}
	l.RecordFailure(key, policy.Limit, policy.Window, policy.Cooldown)
}

func ResetFailure(l *Limiter, key string) {
	l.Reset(key)
}

func IPKey(scope, ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	return scope + ":ip:" + ip
}

func HashedKey(prefix, value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return prefix + hex.EncodeToString(sum[:])
}
