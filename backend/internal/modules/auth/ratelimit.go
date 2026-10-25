package auth

import (
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

const (
	verifyFailureLimit    = 10
	verifyFailureWindow   = 15 * time.Minute
	verifyFailureCooldown = 15 * time.Minute
	verifyIPLimit         = 30
	verifyIPWindow        = 15 * time.Minute
)

type windowPolicy = ratelimit.WindowPolicy
type failurePolicy = ratelimit.FailurePolicy

type verifyPolicies struct {
	VerifyAccount failurePolicy
	VerifyIP      windowPolicy
}

func defaultVerifyPolicies() verifyPolicies {
	return verifyPolicies{
		VerifyAccount: failurePolicy{
			Limit:    verifyFailureLimit,
			Window:   verifyFailureWindow,
			Cooldown: verifyFailureCooldown,
		},
		VerifyIP: windowPolicy{
			Limit:  verifyIPLimit,
			Window: verifyIPWindow,
		},
	}
}

func verifyAccountKey(userID string) string {
	return "verify:user:" + userID
}

func ipKey(scope, ip string) string {
	return ratelimit.IPKey(scope, ip)
}

func (s *Service) allowWindow(policy windowPolicy, key string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, policy, key)
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) checkFailure(policy failurePolicy, key string) error {
	blocked, retryAfter := ratelimit.CheckFailure(s.limiter, policy, key)
	if blocked {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) recordFailure(policy failurePolicy, key string) {
	ratelimit.RecordFailure(s.limiter, policy, key)
}

func (s *Service) resetFailure(key string) {
	ratelimit.ResetFailure(s.limiter, key)
}
