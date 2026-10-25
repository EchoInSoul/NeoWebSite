package auth

import (
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/registration"
)

type rateLimitPolicies struct {
	LoginIdentifier     ratelimit.FailurePolicy
	LoginIP             ratelimit.WindowPolicy
	RegisterEmail       ratelimit.WindowPolicy
	RegisterIP          ratelimit.WindowPolicy
	EmailVerificationIP ratelimit.WindowPolicy
}

func defaultRateLimitPolicies() rateLimitPolicies {
	loginPolicies := login.DefaultPolicies()
	registrationPolicies := registration.DefaultPolicies()
	return rateLimitPolicies{
		LoginIdentifier:     loginPolicies.IdentifierFailure,
		LoginIP:             loginPolicies.IPWindow,
		RegisterEmail:       registrationPolicies.EmailWindow,
		RegisterIP:          registrationPolicies.IPWindow,
		EmailVerificationIP: registrationPolicies.EmailVerificationWindow,
	}
}

func disabledRateLimitPolicies() rateLimitPolicies {
	return rateLimitPolicies{}
}
