package turnstile

import (
	"context"
	"slices"

	"website/backend/internal/common/apierror"
	"website/backend/internal/modules/settings"
)

type Settings interface {
	GetString(ctx context.Context, group, name string) (string, error)
}

type Service struct {
	settings Settings
	verifier Verifier
}

func NewService(settingsService Settings, verifier Verifier) *Service {
	if verifier == nil {
		verifier = unavailableVerifier{}
	}
	return &Service{settings: settingsService, verifier: verifier}
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

	if token == "" {
		return apierror.ErrCaptchaFailed
	}
	if s.verifier.Verify(ctx, secret, token, ip) != Passed {
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
