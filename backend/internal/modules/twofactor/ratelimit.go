package twofactor

import (
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

const (
	failureLimit    = 10
	failureWindow   = 15 * time.Minute
	failureCooldown = 15 * time.Minute
	ipLimit         = 30
	ipWindow        = 15 * time.Minute
)

type Policies struct {
	Account failurePolicy
	IP      windowPolicy
}

type failurePolicy = ratelimit.FailurePolicy
type windowPolicy = ratelimit.WindowPolicy

func DefaultPolicies() Policies {
	return Policies{
		Account: failurePolicy{
			Limit:    failureLimit,
			Window:   failureWindow,
			Cooldown: failureCooldown,
		},
		IP: windowPolicy{
			Limit:  ipLimit,
			Window: ipWindow,
		},
	}
}

func (s *Service) allowIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, s.policies.IP, ipKey("verify", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) checkAccountFailure(key string) error {
	blocked, retryAfter := ratelimit.CheckFailure(s.limiter, s.policies.Account, key)
	if blocked {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) recordAccountFailure(key string) {
	ratelimit.RecordFailure(s.limiter, s.policies.Account, key)
}

func (s *Service) resetAccountFailure(key string) {
	ratelimit.ResetFailure(s.limiter, key)
}

func accountKey(userID string) string {
	return "verify:user:" + userID
}

func ipKey(scope, ip string) string {
	return ratelimit.IPKey(scope, ip)
}
