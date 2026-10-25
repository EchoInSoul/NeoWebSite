package turnstile

import (
	"context"
	"slices"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	captchapolicy "website/backend/internal/modules/captcha/policy"
	"website/backend/internal/modules/settings"
)

type Settings interface {
	GetString(ctx context.Context, group, name string) (string, error)
}

type Policies struct {
	Failure ratelimit.FailurePolicy
}

func DefaultPolicies() Policies {
	return Policies{Failure: captchapolicy.Failure()}
}

type Service struct {
	settings Settings
	verifier Verifier
	limiter  *ratelimit.Limiter
	policies Policies
}

func NewService(settingsService Settings, verifier Verifier) *Service {
	return NewServiceWithPolicies(settingsService, verifier, nil, Policies{})
}

func NewServiceWithPolicies(settingsService Settings, verifier Verifier, limiter *ratelimit.Limiter, policies Policies) *Service {
	if verifier == nil {
		verifier = unavailableVerifier{}
	}
	return &Service{
		settings: settingsService,
		verifier: verifier,
		limiter:  limiter,
		policies: policies,
	}
}

func (s *Service) Verify(ctx context.Context, scope, token, ip string) error {
	secret, _ := s.settings.GetString(ctx, "captcha", "turnstile_secret_key")
	if secret == "" {
		return nil
	}

	scopes, err := s.settings.GetString(ctx, "captcha", "captcha_scopes")
	if err != nil {
		return err
	}
	if !slices.Contains(settings.SplitListValue(scopes), scope) {
		return nil
	}

	blocked, retryAfter := captchapolicy.Allow(s.limiter, s.policies.Failure, ip)
	if blocked {
		return apierror.RateLimited(retryAfter)
	}

	if token == "" {
		captchapolicy.Record(s.limiter, s.policies.Failure, ip)
		return apierror.ErrCaptchaFailed
	}

	outcome := s.verifier.Verify(ctx, secret, token, ip)
	if outcome != Passed {
		if outcome == Rejected {
			captchapolicy.Record(s.limiter, s.policies.Failure, ip)
		}
		return apierror.ErrCaptchaFailed
	}

	return nil
}

func (s *Service) VerifyConfig(ctx context.Context, secret, token, ip string) error {
	switch s.verifier.Verify(ctx, secret, token, ip) {
	case Passed:
		return nil
	case Rejected:
		return apierror.ErrCaptchaVerifyFailed
	default:
		return apierror.ErrServiceUnavailable
	}
}
