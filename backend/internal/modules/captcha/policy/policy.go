package policy

import (
	"time"

	"website/backend/internal/common/ratelimit"
)

const (
	FailureLimit    = 15
	FailureWindow   = 10 * time.Minute
	FailureCooldown = 10 * time.Minute
)

func Failure() ratelimit.FailurePolicy {
	return ratelimit.FailurePolicy{
		Limit:    FailureLimit,
		Window:   FailureWindow,
		Cooldown: FailureCooldown,
	}
}

func FailureKey(ip string) string {
	return ratelimit.IPKey("captcha-fail", ip)
}

func Allow(l *ratelimit.Limiter, failurePolicy ratelimit.FailurePolicy, ip string) (bool, time.Duration) {
	if l == nil {
		return false, 0
	}
	return ratelimit.CheckFailure(l, failurePolicy, FailureKey(ip))
}

func Record(l *ratelimit.Limiter, failurePolicy ratelimit.FailurePolicy, ip string) {
	if l == nil {
		return
	}
	ratelimit.RecordFailure(l, failurePolicy, FailureKey(ip))
}
