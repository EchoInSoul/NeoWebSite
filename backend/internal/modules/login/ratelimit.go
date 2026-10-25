package login

import (
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

const (
	failureLimit    = 5
	failureWindow   = 15 * time.Minute
	failureCooldown = 15 * time.Minute
	ipLimit         = 30
	ipWindow        = 15 * time.Minute
)

type Policies struct {
	IdentifierFailure ratelimit.FailurePolicy
	IPWindow          ratelimit.WindowPolicy
}

func DefaultPolicies() Policies {
	return Policies{
		IdentifierFailure: ratelimit.FailurePolicy{
			Limit:    failureLimit,
			Window:   failureWindow,
			Cooldown: failureCooldown,
		},
		IPWindow: ratelimit.WindowPolicy{
			Limit:  ipLimit,
			Window: ipWindow,
		},
	}
}

func (s *Service) allowIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, s.policies.IPWindow, ratelimit.IPKey("login", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) checkFailure(key string) error {
	blocked, retryAfter := ratelimit.CheckFailure(s.limiter, s.policies.IdentifierFailure, key)
	if blocked {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) recordFailure(key string) {
	ratelimit.RecordFailure(s.limiter, s.policies.IdentifierFailure, key)
}

func (s *Service) resetFailure(key string) {
	ratelimit.ResetFailure(s.limiter, key)
}

func identifierKey(identifier string) string {
	return ratelimit.HashedKey("login:id:", identifier)
}
