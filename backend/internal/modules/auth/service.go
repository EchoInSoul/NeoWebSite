package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/userinput"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/settings"
)

type Service struct {
	repo     *repository
	sessions *session.Service
	settings *settings.Service
	mailer   mail.Mailer
}

func newService(db *database.Database, sessions *session.Service, settingsService *settings.Service, mailer mail.Mailer) *Service {
	if mailer == nil {
		panic("auth: mailer is required")
	}
	return &Service{
		repo:     newRepository(db),
		sessions: sessions,
		settings: settingsService,
		mailer:   mailer,
	}
}

func (s *Service) Logout(ctx context.Context) error {
	return s.sessions.Destroy(ctx)
}

func (s *Service) ChangePassword(ctx context.Context, userID, currentToken, currentPassword, newPassword string) error {
	found, err := s.repo.findUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			s.sessions.Destroy(ctx)
			return apierror.ErrUnauthorized
		}
		return err
	}
	currentVersion := s.sessions.AuthVersion(ctx)

	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return err
	}
	if !userinput.ValidPassword(newPassword, minLength) {
		return apierror.ErrWeakPassword
	}

	if err := verifyPassword(found, currentPassword); err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(newPassword)); err == nil {
		return apierror.ErrPasswordUnchanged
	}

	if err := s.sessions.RevokeOtherSessions(ctx, userID, currentToken); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	updated, err := s.repo.updatePasswordHash(ctx, userID, string(hash))
	if err != nil {
		return err
	}
	if updated.AuthVersion == currentVersion+1 {
		s.sessions.SetAuthVersion(ctx, updated.AuthVersion)
	}
	return nil
}

func (s *Service) ListSessions(ctx context.Context, userID string) ([]session.Session, error) {
	return s.sessions.List(ctx, userID)
}

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
	return s.sessions.Revoke(ctx, userID, sessionID)
}

func (s *Service) CurrentUser(ctx context.Context, userID string) (User, error) {
	found, err := s.repo.findUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			s.sessions.Destroy(ctx)
			return User{}, apierror.ErrUnauthorized
		}
		return User{}, err
	}
	return toUser(found), nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, nickname, email, bio *string) (User, error) {
	if nickname == nil && email == nil && bio == nil {
		return User{}, apierror.ErrInvalidRequest
	}

	var nextNickname, nextEmail, nextBio *string

	if nickname != nil {
		normalized := userinput.NormalizeNickname(*nickname)
		if !userinput.ValidNickname(normalized) {
			return User{}, apierror.WithParam(apierror.ErrInvalidRequest, "nickname")
		}
		nextNickname = &normalized
	}

	if email != nil {
		normalized := userinput.NormalizeEmail(*email)
		if !userinput.ValidEmail(normalized) {
			return User{}, apierror.WithParam(apierror.ErrInvalidRequest, "email")
		}
		nextEmail = &normalized
	}

	if bio != nil {
		normalized := userinput.NormalizeBio(*bio)
		if !userinput.ValidBio(normalized) {
			return User{}, apierror.WithParam(apierror.ErrInvalidRequest, "bio")
		}
		nextBio = &normalized
	}

	if nextNickname != nil {
		if err := s.ensureIdentifierFree(ctx, userID, *nextNickname, apierror.ErrNicknameTaken); err != nil {
			return User{}, err
		}
	}
	if nextEmail != nil {
		if err := s.ensureIdentifierFree(ctx, userID, *nextEmail, apierror.ErrEmailTaken); err != nil {
			return User{}, err
		}
	}

	updated, err := s.repo.updateProfile(ctx, userID, nextNickname, nextEmail, nextBio)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			s.sessions.Destroy(ctx)
			return User{}, apierror.ErrUnauthorized
		}
		if ent.IsConstraintError(err) {
			return User{}, apierror.ErrAlreadyTaken
		}
		return User{}, err
	}

	return toUser(updated), nil
}

func (s *Service) EmailRecoveryCodes(ctx context.Context, userID string, codes []string) error {
	found, err := s.repo.findUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			s.sessions.Destroy(ctx)
			return apierror.ErrUnauthorized
		}
		return err
	}

	configured, err := s.mailer.Configured(ctx)
	if err != nil || !configured {
		return apierror.ErrMailUnavailable
	}

	title, baseURL, err := mail.SiteInfo(ctx, s.settings)
	if err != nil {
		return apierror.ErrMailUnavailable
	}

	subject, body, err := mail.Compose(ctx, s.settings, mail.TemplateRecoveryCodes, map[string]string{
		"siteTitle": title,
		"siteUrl":   baseURL,
		"username":  found.Nickname,
		"codes":     strings.Join(codes, "\n"),
		"sentAt":    time.Now().Format("2006-01-02 15:04:05"),
	})
	if err != nil {
		return apierror.ErrMailUnavailable
	}

	if err := s.mailer.SendSync(ctx, mail.Message{
		To:          found.Email,
		Subject:     subject,
		Body:        body,
		RequireSMTP: true,
	}); err != nil {
		return apierror.ErrMailUnavailable
	}
	return nil
}

func (s *Service) ensureIdentifierFree(ctx context.Context, userID, identifier string, taken error) error {
	found, err := s.repo.findUserByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			return nil
		}
		return err
	}
	if found.ID.String() == userID {
		return nil
	}
	return taken
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

func verifyPassword(found *ent.User, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		return apierror.ErrInvalidCredentials
	}
	return nil
}
