package captcha

import (
	"context"
	"slices"

	"website/backend/internal/common/apierror"
	"website/backend/internal/modules/settings"
)

const providerTurnstile = "turnstile"

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
	GetString(ctx context.Context, group, name string) (string, error)
}

type Service struct {
	settings Settings
	verifier Verifier
}

func New(settings Settings) *Service {
	return NewService(settings, newTurnstileVerifier(turnstileSiteverifyURL, turnstileTimeout))
}

func NewService(settings Settings, verifier Verifier) *Service {
	if verifier == nil {
		verifier = unavailableVerifier{}
	}
	return &Service{settings: settings, verifier: verifier}
}

func (s *Service) Verify(ctx context.Context, scope, token, ip string) error {
	enabled, _ := s.settings.GetBool(ctx, "auth", "captcha_enabled")
	if !enabled {
		return nil
	}

	provider, _ := s.settings.GetString(ctx, "auth", "captcha_provider")
	if provider != providerTurnstile {
		return nil
	}

	secret, _ := s.settings.GetString(ctx, "auth", "turnstile_secret_key")
	if secret == "" {
		return nil
	}

	scopes, err := s.settings.GetString(ctx, "auth", "captcha_scopes")
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
