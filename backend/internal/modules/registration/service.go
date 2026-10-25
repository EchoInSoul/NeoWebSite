package registration

import (
	"context"
	"errors"
	"net/url"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/common/userinput"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/settings"
)

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
	GetString(ctx context.Context, group, name string) (string, error)
	GetNumber(ctx context.Context, group, name string) (int64, error)
	GetGroup(ctx context.Context, group string) (map[string]any, error)
}

type captchaVerifier interface {
	Verify(ctx context.Context, scope, token, ip string) error
}

type Service struct {
	repo     *repository
	settings Settings
	limiter  *ratelimit.Limiter
	policies Policies
	captcha  captchaVerifier
	mailer   mail.Mailer
}

func New(db *database.Database, settingsService Settings, limiter *ratelimit.Limiter, mailer mail.Mailer, captcha captchaVerifier) *Service {
	return NewWithPolicies(db, settingsService, limiter, DefaultPolicies(), mailer, captcha)
}

func NewWithPolicies(db *database.Database, settingsService Settings, limiter *ratelimit.Limiter, policies Policies, mailer mail.Mailer, captcha captchaVerifier) *Service {
	if captcha == nil {
		panic("registration: captcha service is required")
	}
	if mailer == nil {
		panic("registration: mailer is required")
	}
	return &Service{
		repo:     newRepository(db),
		settings: settingsService,
		limiter:  limiter,
		policies: policies,
		captcha:  captcha,
		mailer:   mailer,
	}
}

func (s *Service) Register(ctx context.Context, email, nickname, password, captchaToken string, meta session.Meta) (Result, error) {
	if err := s.allowIP(meta.IP); err != nil {
		return Result{}, err
	}

	enabled, err := s.settings.GetBool(ctx, "register", "enabled")
	if err != nil {
		return Result{}, err
	}
	if !enabled {
		return Result{}, apierror.ErrRegistrationClosed
	}

	email = userinput.NormalizeEmail(email)
	nickname = userinput.NormalizeNickname(nickname)

	if !userinput.ValidEmail(email) || !userinput.ValidNickname(nickname) {
		return Result{}, apierror.ErrInvalidRequest
	}
	if err := s.captcha.Verify(ctx, settings.CaptchaScopeRegister, captchaToken, meta.IP); err != nil {
		return Result{}, err
	}
	if err := s.allowEmail(email); err != nil {
		return Result{}, err
	}
	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return Result{}, err
	}
	if !userinput.ValidPassword(password, minLength) {
		return Result{}, apierror.ErrWeakPassword
	}

	if err := s.ensureRegistrationDomain(ctx, email); err != nil {
		return Result{}, err
	}

	if _, err := s.repo.findUserByIdentifier(ctx, email); err == nil {
		return Result{}, apierror.ErrEmailTaken
	} else if !errors.Is(err, errUserNotFound) {
		return Result{}, err
	}
	if _, err := s.repo.findUserByIdentifier(ctx, nickname); err == nil {
		return Result{}, apierror.ErrNicknameTaken
	} else if !errors.Is(err, errUserNotFound) {
		return Result{}, err
	}

	verificationEnabled, err := s.settings.GetBool(ctx, "register", "email_verification_enabled")
	if err != nil {
		return Result{}, err
	}

	if !verificationEnabled {
		hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return Result{}, err
		}
		created, err := s.repo.createUser(ctx, email, nickname, string(hash), emailVerificationNotRequired)
		if err != nil {
			if ent.IsConstraintError(err) {
				return Result{}, apierror.ErrAlreadyTaken
			}
			return Result{}, err
		}
		user := toUser(created)
		return Result{User: &user}, nil
	}

	configured, err := s.mailer.Configured(ctx)
	if err != nil || !configured {
		return Result{}, apierror.ErrMailUnavailable
	}
	data, err := s.registrationEmailData(ctx, nickname)
	if err != nil {
		return Result{}, apierror.ErrMailUnavailable
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return Result{}, err
	}
	rawToken, tokenHash, err := newPendingToken()
	if err != nil {
		return Result{}, err
	}
	record, err := s.repo.replacePendingRegistration(ctx, pendingRegistrationInput{
		Email:        email,
		Nickname:     nickname,
		PasswordHash: string(hash),
		TokenHash:    tokenHash,
		ExpiresAt:    time.Now().Add(pendingRegistrationTTL),
	})
	if err != nil {
		return Result{}, err
	}

	data["verifyUrl"] = data["siteUrl"] + "/register/verify?token=" + url.QueryEscape(rawToken)
	subject, body, err := mail.Compose(ctx, s.settings, mail.TemplateSignupCode, data)
	if err != nil {
		s.compensatePending(ctx, record)
		return Result{}, apierror.ErrMailUnavailable
	}
	if err := s.mailer.SendSync(ctx, mail.Message{To: email, Subject: subject, Body: body, RequireSMTP: true}); err != nil {
		s.compensatePending(ctx, record)
		return Result{}, apierror.ErrMailUnavailable
	}

	return Result{VerificationPending: true}, nil
}

func (s *Service) VerifyEmail(ctx context.Context, token string, meta session.Meta) (User, error) {
	if err := s.allowEmailVerificationIP(meta.IP); err != nil {
		return User{}, err
	}

	created, err := s.repo.verifyPendingRegistration(ctx, hashPendingToken(token))
	if err != nil {
		switch {
		case errors.Is(err, errPendingNotFound):
			return User{}, apierror.ErrEmailVerificationInvalid
		case errors.Is(err, errPendingEmailTaken):
			return User{}, apierror.ErrEmailTaken
		case errors.Is(err, errPendingNicknameTaken):
			return User{}, apierror.ErrNicknameTaken
		case errors.Is(err, errPendingConflict):
			return User{}, apierror.ErrAlreadyTaken
		default:
			return User{}, err
		}
	}
	return toUser(created), nil
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

func (s *Service) ensureRegistrationDomain(ctx context.Context, email string) error {
	controlEnabled, err := s.settings.GetBool(ctx, "register", "email_domain_control_enabled")
	if err != nil {
		return err
	}
	if !controlEnabled {
		return nil
	}
	raw, err := s.settings.GetString(ctx, "register", "allowed_email_domains")
	if err != nil {
		return err
	}
	allowed := settings.SplitListValue(raw)
	if len(allowed) == 0 {
		return nil
	}
	if userinput.EmailDomainAllowed(email, allowed) {
		return nil
	}
	return apierror.ErrEmailDomainNotAllowed
}
