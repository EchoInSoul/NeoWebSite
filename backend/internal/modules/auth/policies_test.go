package auth

import (
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/registration"
)

type rateLimitPolicies struct {
	LoginIdentifier     failurePolicy
	LoginIP             windowPolicy
	RegisterEmail       windowPolicy
	RegisterIP          windowPolicy
	EmailVerificationIP windowPolicy
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
