package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/settings"
)

const (
	pendingUserIDKey   = "auth.pending_2fa_user_id"
	pendingAttemptsKey = "auth.pending_2fa_attempts"
	maxVerifyAttempts  = 5
)

const recoveryCodesPerBatch = 5

type Service struct {
	repo      *repository
	sessions  *scs.SessionManager
	dummyHash []byte
	totpKey   []byte
	settings  *settings.Service
	limiter   *ratelimit.Limiter
	policies  rateLimitPolicies
	captcha   captchaVerifier
}

func newService(db *database.Database, sessions *scs.SessionManager, totpKey []byte, settingsService *settings.Service, limiter *ratelimit.Limiter, policies rateLimitPolicies, verifier captchaVerifier) *Service {
	dummy, err := bcrypt.GenerateFromPassword([]byte("timing-defense-placeholder"), bcrypt.DefaultCost)
	if err != nil {
		panic(fmt.Sprintf("auth: generate dummy hash: %v", err))
	}
	if verifier == nil {
		verifier = unavailableCaptchaVerifier{}
	}
	return &Service{
		repo:      newRepository(db),
		sessions:  sessions,
		dummyHash: dummy,
		totpKey:   totpKey,
		settings:  settingsService,
		limiter:   limiter,
		policies:  policies,
		captcha:   verifier,
	}
}

func (s *Service) Register(ctx context.Context, email, nickname, password, captchaToken string, meta SessionMeta) (User, error) {
	if err := s.allowWindow(s.policies.RegisterIP, ipKey("register", meta.IP)); err != nil {
		return User{}, err
	}

	enabled, err := s.settings.GetBool(ctx, "register", "enabled")
	if err != nil {
		return User{}, err
	}
	if !enabled {
		return User{}, apierror.ErrRegistrationClosed
	}

	email = normalizeEmail(email)
	nickname = normalizeNickname(nickname)

	if !validEmail(email) || !validNickname(nickname) {
		return User{}, apierror.ErrInvalidRequest
	}
	if err := s.verifyCaptcha(ctx, captchaToken, meta); err != nil {
		return User{}, err
	}
	if err := s.allowWindow(s.policies.RegisterEmail, registerEmailKey(email)); err != nil {
		return User{}, err
	}
	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return User{}, err
	}
	if !validPassword(password, minLength) {
		return User{}, apierror.ErrWeakPassword
	}

	if err := s.ensureRegistrationDomain(ctx, email); err != nil {
		return User{}, err
	}

	if _, err := s.repo.findUserByIdentifier(ctx, email); err == nil {
		return User{}, apierror.ErrEmailTaken
	} else if !errors.Is(err, errUserNotFound) {
		return User{}, err
	}
	if _, err := s.repo.findUserByIdentifier(ctx, nickname); err == nil {
		return User{}, apierror.ErrNicknameTaken
	} else if !errors.Is(err, errUserNotFound) {
		return User{}, err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	created, err := s.repo.createUser(ctx, email, nickname, string(hash))
	if err != nil {
		if ent.IsConstraintError(err) {
			return User{}, apierror.ErrAlreadyTaken
		}
		return User{}, err
	}

	return toUser(created), nil
}

func (s *Service) Login(ctx context.Context, identifier, password, captchaToken string, meta SessionMeta) (User, error) {
	if err := s.allowWindow(s.policies.LoginIP, ipKey("login", meta.IP)); err != nil {
		return User{}, err
	}
	accountKey := loginIdentifierKey(identifier)
	if err := s.checkFailure(s.policies.LoginIdentifier, accountKey); err != nil {
		return User{}, err
	}
	if err := s.verifyCaptcha(ctx, captchaToken, meta); err != nil {
		return User{}, err
	}

	if len(password) > maxPasswordBytes {
		return User{}, apierror.ErrInvalidCredentials
	}

	found, err := s.repo.findUserByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
			s.recordFailure(s.policies.LoginIdentifier, accountKey)
			return User{}, apierror.ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		s.recordFailure(s.policies.LoginIdentifier, accountKey)
		return User{}, apierror.ErrInvalidCredentials
	}
	s.resetFailure(accountKey)

	if found.TotpEnabledAt != nil {
		if err := s.sessions.RenewToken(ctx); err != nil {
			return User{}, err
		}
		s.enterPendingSession(ctx, found.ID.String())
		return User{}, apierror.ErrTwoFactorRequired
	}

	if err := s.establishSession(ctx, found.ID.String(), found.AuthVersion, meta); err != nil {
		return User{}, err
	}

	return toUser(found), nil
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
	currentVersion := s.sessions.GetInt(ctx, guard.SessionAuthVersionKey)

	minLength, err := s.passwordMinLength(ctx)
	if err != nil {
		return err
	}
	if !validPassword(newPassword, minLength) {
		return apierror.ErrWeakPassword
	}

	if err := verifyPassword(found, currentPassword); err != nil {
		return err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(newPassword)); err == nil {
		return apierror.ErrPasswordUnchanged
	}

	if err := s.revokeOtherSessions(userID, currentToken); err != nil {
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
		s.sessions.Put(ctx, guard.SessionAuthVersionKey, updated.AuthVersion)
	}
	return nil
}

func (s *Service) revokeOtherSessions(userID, currentToken string) error {
	return s.eachSession(func(token string, values map[string]any, _ time.Time) error {
		if token == currentToken {
			return nil
		}
		if sessionString(values, guard.SessionUserIDKey) != userID {
			return nil
		}
		return s.sessions.Store.Delete(token)
	})
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
		normalized := normalizeNickname(*nickname)
		if !validNickname(normalized) {
			return User{}, apierror.WithParam(apierror.ErrInvalidRequest, "nickname")
		}
		nextNickname = &normalized
	}

	if email != nil {
		normalized := normalizeEmail(*email)
		if !validEmail(normalized) {
			return User{}, apierror.WithParam(apierror.ErrInvalidRequest, "email")
		}
		nextEmail = &normalized
	}

	if bio != nil {
		normalized := normalizeBio(*bio)
		if !validBio(normalized) {
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

func isFreshTOTPSetup(row *ent.User, now time.Time) bool {
	return row.TotpSecret != nil && row.TotpSetupAt != nil && now.Sub(*row.TotpSetupAt) < totpSetupReuseWindow
}

func (s *Service) SetupTOTP(ctx context.Context, userID, password string) (TwoFactorSetupResponse, error) {
	available, err := s.twoFactorAvailable(ctx)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}
	if !available {
		return TwoFactorSetupResponse{}, apierror.ErrTwoFactorDisabled
	}
	found, err := s.activeUser(ctx, userID)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}
	if err := verifyPassword(found, password); err != nil {
		return TwoFactorSetupResponse{}, err
	}
	if found.TotpEnabledAt != nil {
		return TwoFactorSetupResponse{}, apierror.ErrTwoFactorAlreadyEnabled
	}

	now := totpNow()
	if isFreshTOTPSetup(found, now) {
		return s.reuseTOTPSetup(found.Email, *found.TotpSecret)
	}

	setup, err := generateTOTPSetup(found.Email)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}
	sealed, err := sealSecret(s.totpKey, setup.Secret)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}

	for attempt := 0; attempt < totpSetupClaimAttempts; attempt++ {
		claimed, err := s.repo.claimTOTPSetup(ctx, found.ID, sealed, now)
		if err != nil {
			return TwoFactorSetupResponse{}, err
		}
		if claimed {
			return TwoFactorSetupResponse{Secret: setup.Secret, OTPAuthURL: setup.OTPAuthURL}, nil
		}

		current, err := s.activeUser(ctx, userID)
		if err != nil {
			return TwoFactorSetupResponse{}, err
		}
		if current.TotpEnabledAt != nil {
			return TwoFactorSetupResponse{}, apierror.ErrTwoFactorAlreadyEnabled
		}
		if isFreshTOTPSetup(current, now) {
			return s.reuseTOTPSetup(current.Email, *current.TotpSecret)
		}
	}
	return TwoFactorSetupResponse{}, apierror.ErrInternal
}

func (s *Service) reuseTOTPSetup(accountName, sealed string) (TwoFactorSetupResponse, error) {
	plain, err := openSecret(s.totpKey, sealed)
	if err != nil {
		return TwoFactorSetupResponse{}, apierror.ErrInternal
	}
	setup, err := rebuildTOTPSetup(accountName, plain)
	if err != nil {
		return TwoFactorSetupResponse{}, apierror.ErrInternal
	}
	return TwoFactorSetupResponse{Secret: setup.Secret, OTPAuthURL: setup.OTPAuthURL}, nil
}

func (s *Service) EnableTOTP(ctx context.Context, userID, code, password, currentToken string) (RecoveryCodesResponse, error) {
	available, err := s.twoFactorAvailable(ctx)
	if err != nil {
		return RecoveryCodesResponse{}, err
	}
	if !available {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorDisabled
	}
	found, err := s.activeUser(ctx, userID)
	if err != nil {
		return RecoveryCodesResponse{}, err
	}
	currentVersion := s.sessions.GetInt(ctx, guard.SessionAuthVersionKey)
	if err := verifyPassword(found, password); err != nil {
		return RecoveryCodesResponse{}, err
	}
	if found.TotpSecret == nil {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorNotSetup
	}
	if found.TotpEnabledAt != nil {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorAlreadyEnabled
	}

	secret, err := openSecret(s.totpKey, *found.TotpSecret)
	if err != nil {
		return RecoveryCodesResponse{}, apierror.ErrInternal
	}

	now := totpNow()
	counter, matched, err := matchTOTPCounter(code, secret, now)
	if err != nil {
		return RecoveryCodesResponse{}, apierror.ErrInternal
	}
	if !matched {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorInvalidCode
	}

	if err := s.revokeOtherSessions(userID, currentToken); err != nil {
		return RecoveryCodesResponse{}, err
	}

	claimed, err := s.repo.claimTOTPCounter(ctx, found.ID, counter)
	if err != nil {
		return RecoveryCodesResponse{}, err
	}
	if !claimed {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorInvalidCode
	}

	codes, hashes, err := issueRecoveryCodes()
	if err != nil {
		return RecoveryCodesResponse{}, err
	}
	newVersion, err := s.repo.enableTOTP(ctx, found.ID, now, hashes)
	if err != nil {
		return RecoveryCodesResponse{}, err
	}
	if newVersion == currentVersion+1 {
		s.sessions.Put(ctx, guard.SessionAuthVersionKey, newVersion)
	}
	return RecoveryCodesResponse{RecoveryCodes: codes}, nil
}

func (s *Service) DisableTOTP(ctx context.Context, userID, password, currentToken string) error {
	found, err := s.activeUser(ctx, userID)
	if err != nil {
		return err
	}
	if found.TotpSecret == nil {
		return apierror.ErrTwoFactorNotSetup
	}
	if err := verifyPassword(found, password); err != nil {
		return err
	}

	if err := s.revokeOtherSessions(userID, currentToken); err != nil {
		return err
	}
	return s.repo.disableTOTP(ctx, found.ID)
}

func (s *Service) RegenerateRecoveryCodes(ctx context.Context, userID, password string) (RecoveryCodesResponse, error) {
	found, err := s.activeUser(ctx, userID)
	if err != nil {
		return RecoveryCodesResponse{}, err
	}
	if found.TotpEnabledAt == nil {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorNotEnabled
	}
	if err := verifyPassword(found, password); err != nil {
		return RecoveryCodesResponse{}, err
	}

	codes, hashes, err := issueRecoveryCodes()
	if err != nil {
		return RecoveryCodesResponse{}, err
	}
	if err := s.repo.replaceRecoveryCodes(ctx, found.ID, hashes); err != nil {
		return RecoveryCodesResponse{}, err
	}
	return RecoveryCodesResponse{RecoveryCodes: codes}, nil
}

func (s *Service) twoFactorAvailable(ctx context.Context) (bool, error) {
	enabled, err := s.settings.GetBool(ctx, "auth", "totp_enabled")
	if err != nil {
		return false, err
	}
	return enabled, nil
}

func (s *Service) activeUser(ctx context.Context, userID string) (*ent.User, error) {
	found, err := s.repo.findUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			s.sessions.Destroy(ctx)
			return nil, apierror.ErrUnauthorized
		}
		return nil, err
	}
	return found, nil
}

func verifyPassword(found *ent.User, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		return apierror.ErrInvalidCredentials
	}
	return nil
}

func issueRecoveryCodes() ([]string, []string, error) {
	codes, err := generateRecoveryCodes(recoveryCodesPerBatch)
	if err != nil {
		return nil, nil, err
	}

	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hashes = append(hashes, hashRecoveryCode(code))
	}
	return codes, hashes, nil
}

func (s *Service) VerifySecondFactor(ctx context.Context, code string, meta SessionMeta) (User, error) {
	if err := s.allowWindow(s.policies.VerifyIP, ipKey("verify", meta.IP)); err != nil {
		return User{}, err
	}

	pendingUserID := s.sessions.GetString(ctx, pendingUserIDKey)
	if pendingUserID == "" {
		return User{}, apierror.ErrUnauthorized
	}

	accountKey := verifyAccountKey(pendingUserID)
	if err := s.checkFailure(s.policies.VerifyAccount, accountKey); err != nil {
		return User{}, err
	}

	found, err := s.repo.findUserByID(ctx, pendingUserID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			s.sessions.Destroy(ctx)
			return User{}, apierror.ErrUnauthorized
		}
		return User{}, err
	}
	if found.TotpEnabledAt == nil {
		s.sessions.Destroy(ctx)
		return User{}, apierror.ErrUnauthorized
	}

	verified, err := s.matchTOTP(ctx, found, code)
	if err != nil {
		return User{}, err
	}
	if !verified {
		consumed, err := s.repo.consumeRecoveryCode(ctx, found.ID, hashRecoveryCode(code))
		if err != nil {
			return User{}, err
		}
		verified = consumed
	}
	if !verified {
		s.recordFailure(s.policies.VerifyAccount, accountKey)
		return User{}, s.failSecondFactor(ctx)
	}
	s.resetFailure(accountKey)

	if err := s.establishSession(ctx, found.ID.String(), found.AuthVersion, meta); err != nil {
		return User{}, err
	}

	return toUser(found), nil
}

func (s *Service) matchTOTP(ctx context.Context, found *ent.User, code string) (bool, error) {
	if found.TotpSecret == nil {
		return false, apierror.ErrInternal
	}

	secret, err := openSecret(s.totpKey, *found.TotpSecret)
	if err != nil {
		return false, apierror.ErrInternal
	}

	counter, matched, err := matchTOTPCounter(code, secret, totpNow())
	if err != nil {
		return false, apierror.ErrInternal
	}
	if !matched {
		return false, nil
	}

	claimed, err := s.repo.claimTOTPCounter(ctx, found.ID, counter)
	if err != nil {
		return false, err
	}
	return claimed, nil
}

func (s *Service) failSecondFactor(ctx context.Context) error {
	attempts := s.sessions.GetInt(ctx, pendingAttemptsKey) + 1
	if attempts >= maxVerifyAttempts {
		s.sessions.Destroy(ctx)
		return apierror.ErrTwoFactorLocked
	}

	s.sessions.Put(ctx, pendingAttemptsKey, attempts)
	return apierror.ErrTwoFactorInvalidCode
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
	return effectivePasswordMinLength(requirementsEnabled, configured), nil
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
	if emailDomainAllowed(email, allowed) {
		return nil
	}
	return apierror.ErrEmailDomainNotAllowed
}

func (s *Service) verifyCaptcha(ctx context.Context, token string, meta SessionMeta) error {
	enabled, _ := s.settings.GetBool(ctx, "auth", "captcha_enabled")
	if !enabled {
		return nil
	}

	provider, _ := s.settings.GetString(ctx, "auth", "captcha_provider")
	if provider != captchaProviderTurnstile {
		return nil
	}

	secret, _ := s.settings.GetString(ctx, "auth", "turnstile_secret_key")
	if secret == "" {
		return nil
	}

	if token == "" {
		return apierror.ErrCaptchaFailed
	}
	if s.captcha.Verify(ctx, secret, token, meta.IP) != captchaPassed {
		return apierror.ErrCaptchaFailed
	}

	return nil
}

func (s *Service) VerifyCaptchaConfig(ctx context.Context, secret, token string, meta SessionMeta) error {
	switch s.captcha.Verify(ctx, secret, token, meta.IP) {
	case captchaPassed:
		return nil
	case captchaRejected:
		return apierror.ErrCaptchaVerifyFailed
	default:
		return apierror.ErrServiceUnavailable
	}
}
