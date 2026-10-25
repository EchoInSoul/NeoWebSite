package login

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/common/userinput"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/settings"
)

type captchaVerifier interface {
	Verify(ctx context.Context, scope, token, ip string) error
}

type Service struct {
	repo      *repository
	sessions  *session.Service
	limiter   *ratelimit.Limiter
	policies  Policies
	captcha   captchaVerifier
	dummyHash []byte
}

func New(db *database.Database, sessions *session.Service, limiter *ratelimit.Limiter, captcha captchaVerifier) *Service {
	return NewWithPolicies(db, sessions, limiter, DefaultPolicies(), captcha)
}

func NewWithPolicies(db *database.Database, sessions *session.Service, limiter *ratelimit.Limiter, policies Policies, captcha captchaVerifier) *Service {
	if captcha == nil {
		panic("login: captcha service is required")
	}
	dummy, err := bcrypt.GenerateFromPassword([]byte("timing-defense-placeholder"), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("login: generate dummy hash: %v", err))
	}
	return &Service{
		repo:      newRepository(db),
		sessions:  sessions,
		limiter:   limiter,
		policies:  policies,
		captcha:   captcha,
		dummyHash: dummy,
	}
}

func (s *Service) Login(ctx context.Context, identifier, password, captchaToken string, meta session.Meta) (User, error) {
	if err := s.allowIP(meta.IP); err != nil {
		return User{}, err
	}
	accountKey := identifierKey(identifier)
	if err := s.checkFailure(accountKey); err != nil {
		return User{}, err
	}
	if err := s.captcha.Verify(ctx, settings.CaptchaScopeLogin, captchaToken, meta.IP); err != nil {
		return User{}, err
	}

	if len(password) > userinput.PasswordMaxBytes {
		return User{}, apierror.ErrInvalidCredentials
	}

	found, err := s.repo.findUserByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			if strings.Contains(identifier, "@") {
				pending, pendingErr := s.repo.findPendingRegistrationByEmail(ctx, userinput.NormalizeEmail(identifier))
				if pendingErr == nil && pending.ExpiresAt.After(time.Now()) {
					if bcrypt.CompareHashAndPassword([]byte(pending.PasswordHash), []byte(password)) == nil {
						s.resetFailure(accountKey)
						return User{}, apierror.ErrEmailNotVerified
					}
					s.recordFailure(accountKey)
					return User{}, apierror.ErrInvalidCredentials
				}
				if pendingErr != nil && !errors.Is(pendingErr, errPendingNotFound) {
					return User{}, pendingErr
				}
			}
			_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
			s.recordFailure(accountKey)
			return User{}, apierror.ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		s.recordFailure(accountKey)
		return User{}, apierror.ErrInvalidCredentials
	}
	s.resetFailure(accountKey)

	if found.TotpEnabledAt != nil {
		if err := s.sessions.EnterPending2FA(ctx, found.ID.String()); err != nil {
			return User{}, err
		}
		return User{}, apierror.ErrTwoFactorRequired
	}

	if err := s.sessions.Establish(ctx, found.ID.String(), found.AuthVersion, meta); err != nil {
		return User{}, err
	}

	return toUser(found), nil
}

func toUser(u *ent.User) User {
	return User{
		Email:    u.Email,
		Nickname: u.Nickname,
		Bio:      u.Bio,
	}
}
