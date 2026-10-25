package captcha

import (
	"context"

	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/modules/captcha/builtin"
	"website/backend/internal/modules/captcha/turnstile"
)

const (
	providerTurnstile = "turnstile"
	providerBuiltin   = "builtin"
)

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
	GetString(ctx context.Context, group, name string) (string, error)
}

type Service struct {
	settings  Settings
	turnstile *turnstile.Service
	builtin   *builtin.Service
}

func NewService(settings Settings, verifier Verifier) *Service {
	return newService(settings, turnstile.NewService(settings, verifier), nil)
}

func newService(settingsService Settings, turnstileService *turnstile.Service, builtinService *builtin.Service) *Service {
	return &Service{settings: settingsService, turnstile: turnstileService, builtin: builtinService}
}

func (s *Service) Verify(ctx context.Context, scope, token, ip string) error {
	enabled, _ := s.settings.GetBool(ctx, "captcha", "captcha_enabled")
	if !enabled {
		return nil
	}

	provider, _ := s.settings.GetString(ctx, "captcha", "captcha_provider")
	switch provider {
	case providerTurnstile:
		if s.turnstile == nil {
			return apierror.ErrServiceUnavailable
		}
		return s.turnstile.Verify(ctx, scope, token, ip)
	case providerBuiltin:
		if s.builtin == nil {
			return apierror.ErrServiceUnavailable
		}
		return s.builtin.VerifyToken(ctx, scope, token)
	default:
		return nil
	}
}

func (s *Service) Challenge(ctx context.Context, scope, ip string) (builtin.ChallengeResponse, error) {
	if s.builtin == nil {
		return builtin.ChallengeResponse{}, apierror.ErrServiceUnavailable
	}
	return s.builtin.Challenge(ctx, scope, ip)
}

func (s *Service) Solve(ctx context.Context, id uuid.UUID, x, y int) (builtin.SolveChallengeResponse, error) {
	if s.builtin == nil {
		return builtin.SolveChallengeResponse{}, apierror.ErrServiceUnavailable
	}
	return s.builtin.Solve(ctx, id, x, y)
}
