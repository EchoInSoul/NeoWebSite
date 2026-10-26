package passwordreset

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/common/userinput"
	"website/backend/internal/database"
	"website/backend/internal/modules/mail"
)

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
	GetNumber(ctx context.Context, group, name string) (int64, error)
	GetGroup(ctx context.Context, group string) (map[string]any, error)
}

type Service struct {
	repo     *repository
	settings Settings
	limiter  *ratelimit.Limiter
	mailer   mail.Mailer
}

func New(db *database.Database, settings Settings, limiter *ratelimit.Limiter, mailer mail.Mailer) *Service {
	if settings == nil {
		panic("passwordreset: settings is required")
	}
	if limiter == nil {
		panic("passwordreset: limiter is required")
	}
	if mailer == nil {
		panic("passwordreset: mailer is required")
	}
	return &Service{
		repo:     newRepository(db),
		settings: settings,
		limiter:  limiter,
		mailer:   mailer,
	}
}

func (s *Service) Request(ctx context.Context, email, ip string) error {
	if err := s.allowRequestIP(ip); err != nil {
		return err
	}

	enabled, err := s.settings.GetBool(ctx, "auth", "password_reset_enabled")
	if err != nil {
		return err
	}
	if !enabled {
		return apierror.ErrPasswordResetDisabled
	}

	email = userinput.NormalizeEmail(email)
	if !userinput.ValidEmail(email) {
		return apierror.WithParam(apierror.ErrInvalidRequest, "email")
	}
	if err := s.allowRequestEmail(email); err != nil {
		return err
	}

	configured, err := s.mailer.Configured(ctx)
	if err != nil || !configured {
		return apierror.ErrMailUnavailable
	}
	title, baseURL, err := s.siteInfo(ctx)
	if err != nil {
		return apierror.ErrMailUnavailable
	}

	user, err := s.repo.findUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			return nil
		}
		return err
	}

	rawToken, tokenHash, err := newToken()
	if err != nil {
		return err
	}
	resetID, err := s.repo.replaceReset(ctx, user.ID, tokenHash, time.Now().Add(resetTTL))
	if err != nil {
		return err
	}

	subject, body, err := mail.Compose(ctx, s.settings, mail.TemplatePasswordReset, resetEmailData(title, baseURL, user, rawToken))
	if err != nil {
		s.compensate(ctx, resetID, tokenHash)
		return apierror.ErrMailUnavailable
	}
	if err := s.mailer.Send(ctx, mail.Message{
		To:          user.Email,
		Subject:     subject,
		Body:        body,
		RequireSMTP: true,
	}); err != nil {
		s.compensate(ctx, resetID, tokenHash)
		return apierror.ErrMailUnavailable
	}

	return nil
}

func (s *Service) compensate(ctx context.Context, id uuid.UUID, tokenHash string) {
	if _, err := s.repo.deleteReset(ctx, id, tokenHash); err != nil {
		slog.Error("passwordreset: delete reset", "err", err)
	}
}

func (s *Service) passwordMinLength(ctx context.Context) (int, error) {
	requirementsEnabled, err := s.settings.GetBool(ctx, "auth", "password_requirements_enabled")
	if err != nil {
		return 0, err
	}

	configured := int64(0)
	if requirementsEnabled {
		configured, err = s.settings.GetNumber(ctx, "auth", "password_min_length")
		if err != nil {
			return 0, err
		}
	}
	return userinput.EffectivePasswordMinLength(requirementsEnabled, configured), nil
}

func (s *Service) Confirm(ctx context.Context, token, newPassword, ip string) error {
	if err := s.allowConfirmIP(ip); err != nil {
		return err
	}

	enabled, err := s.settings.GetBool(ctx, "auth", "password_reset_enabled")
	if err != nil {
		return err
	}
	if !enabled {
		return apierror.ErrPasswordResetDisabled
	}

	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return err
	}
	if !userinput.ValidPassword(newPassword, minLength) {
		return apierror.WithParam(apierror.ErrWeakPassword, "new_password")
	}

	tokenHash := hashToken(token)
	reset, err := s.repo.findReset(ctx, tokenHash)
	if err != nil {
		if errors.Is(err, errResetNotFound) {
			return apierror.ErrPasswordResetInvalid
		}
		return err
	}
	if !reset.ExpiresAt.After(time.Now()) {
		if _, err := s.repo.deleteReset(ctx, reset.ID, tokenHash); err != nil {
			return err
		}
		return apierror.ErrPasswordResetInvalid
	}

	user, err := s.repo.findUserByID(ctx, reset.UserID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			return apierror.ErrPasswordResetInvalid
		}
		return err
	}
	if bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(newPassword)) == nil {
		return apierror.ErrPasswordUnchanged
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.completeReset(ctx, reset.ID, tokenHash, user.ID, string(newHash)); err != nil {
		if errors.Is(err, errResetNotFound) || errors.Is(err, errUserNotFound) {
			return apierror.ErrPasswordResetInvalid
		}
		return err
	}
	return nil
}
