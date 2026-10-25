package registration

import (
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

const (
	emailLimit                = 3
	emailWindow               = time.Hour
	ipLimit                   = 5
	ipWindow                  = time.Hour
	emailVerificationIPLimit  = 30
	emailVerificationIPWindow = 15 * time.Minute
)

type Policies struct {
	EmailWindow             ratelimit.WindowPolicy
	IPWindow                ratelimit.WindowPolicy
	EmailVerificationWindow ratelimit.WindowPolicy
}

func DefaultPolicies() Policies {
	return Policies{
		EmailWindow: ratelimit.WindowPolicy{
			Limit:  emailLimit,
			Window: emailWindow,
		},
		IPWindow: ratelimit.WindowPolicy{
			Limit:  ipLimit,
			Window: ipWindow,
		},
		EmailVerificationWindow: ratelimit.WindowPolicy{
			Limit:  emailVerificationIPLimit,
			Window: emailVerificationIPWindow,
		},
	}
}

func (s *Service) allowIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, s.policies.IPWindow, ratelimit.IPKey("register", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) allowEmail(email string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, s.policies.EmailWindow, emailKey(email))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) allowEmailVerificationIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, s.policies.EmailVerificationWindow, ratelimit.IPKey("emailverify", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func emailKey(email string) string {
	return ratelimit.HashedKey("register:email:", email)
}
