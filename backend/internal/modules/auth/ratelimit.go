package auth

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"website/backend/internal/common/apierror"
)

const (
	loginFailureLimit     = 5
	loginFailureWindow    = 15 * time.Minute
	loginFailureCooldown  = 15 * time.Minute
	loginIPLimit          = 30
	loginIPWindow         = 15 * time.Minute
	registerEmailLimit    = 3
	registerEmailWindow   = time.Hour
	registerIPLimit       = 5
	registerIPWindow      = time.Hour
	verifyFailureLimit    = 10
	verifyFailureWindow   = 15 * time.Minute
	verifyFailureCooldown = 15 * time.Minute
	verifyIPLimit         = 30
	verifyIPWindow        = 15 * time.Minute
)

type windowPolicy struct {
	Limit  int
	Window time.Duration
}

type failurePolicy struct {
	Limit    int
	Window   time.Duration
	Cooldown time.Duration
}

type rateLimitPolicies struct {
	LoginIdentifier failurePolicy
	LoginIP         windowPolicy
	RegisterEmail   windowPolicy
	RegisterIP      windowPolicy
	VerifyAccount   failurePolicy
	VerifyIP        windowPolicy
}

func defaultRateLimitPolicies() rateLimitPolicies {
	return rateLimitPolicies{
		LoginIdentifier: failurePolicy{
			Limit:    loginFailureLimit,
			Window:   loginFailureWindow,
			Cooldown: loginFailureCooldown,
		},
		LoginIP: windowPolicy{
			Limit:  loginIPLimit,
			Window: loginIPWindow,
		},
		RegisterEmail: windowPolicy{
			Limit:  registerEmailLimit,
			Window: registerEmailWindow,
		},
		RegisterIP: windowPolicy{
			Limit:  registerIPLimit,
			Window: registerIPWindow,
		},
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

func disabledRateLimitPolicies() rateLimitPolicies {
	return rateLimitPolicies{}
}

func loginIdentifierKey(identifier string) string {
	return hashedRateLimitKey("login:id:", identifier)
}

func registerEmailKey(email string) string {
	return hashedRateLimitKey("register:email:", email)
}

func verifyAccountKey(userID string) string {
	return "verify:user:" + userID
}

func ipKey(scope, ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		ip = "unknown"
	}
	return scope + ":ip:" + ip
}

func hashedRateLimitKey(prefix, value string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return prefix + hex.EncodeToString(sum[:])
}

func (s *Service) allowWindow(policy windowPolicy, key string) error {
	if policy.Limit <= 0 {
		return nil
	}
	allowed, retryAfter := s.limiter.Allow(key, policy.Limit, policy.Window)
	if !allowed {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) checkFailure(policy failurePolicy, key string) error {
	if policy.Limit <= 0 {
		return nil
	}
	blocked, retryAfter := s.limiter.Blocked(key)
	if blocked {
		return apierror.RateLimited(retryAfter)
	}
	return nil
}

func (s *Service) recordFailure(policy failurePolicy, key string) {
	if policy.Limit <= 0 {
		return
	}
	s.limiter.RecordFailure(key, policy.Limit, policy.Window, policy.Cooldown)
}

func (s *Service) resetFailure(key string) {
	s.limiter.Reset(key)
}
