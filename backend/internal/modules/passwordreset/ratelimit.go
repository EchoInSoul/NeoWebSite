package passwordreset

import (
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

const (
	requestIPLimit     = 5
	requestIPWindow    = time.Hour
	requestEmailLimit  = 3
	requestEmailWindow = time.Hour
	confirmIPLimit     = 30
	confirmIPWindow    = 15 * time.Minute
)

func (s *Service) allowRequestIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, ratelimit.WindowPolicy{
		Limit:  requestIPLimit,
		Window: requestIPWindow,
	}, ratelimit.IPKey("passwordreset", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) allowRequestEmail(email string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, ratelimit.WindowPolicy{
		Limit:  requestEmailLimit,
		Window: requestEmailWindow,
	}, ratelimit.HashedKey("passwordreset:email:", email))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) allowConfirmIP(ip string) error {
	allowed, retryAfter := ratelimit.AllowWindow(s.limiter, ratelimit.WindowPolicy{
		Limit:  confirmIPLimit,
		Window: confirmIPWindow,
	}, ratelimit.IPKey("passwordreset-confirm", ip))
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}
