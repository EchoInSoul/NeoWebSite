package twofactor

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/session"
)

const (
	maxVerifyAttempts          = 5
	maxRecoveryCodeInputLength = 64
)

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
}

type Service struct {
	repo     *repository
	sessions *session.Service
	settings Settings
	limiter  *ratelimit.Limiter
	policies Policies
	totpKey  []byte
}

func New(db *database.Database, sessions *session.Service, settingsService Settings, limiter *ratelimit.Limiter, totpKey []byte) *Service {
	return NewWithPolicies(db, sessions, settingsService, limiter, DefaultPolicies(), totpKey)
}

func NewWithPolicies(db *database.Database, sessions *session.Service, settingsService Settings, limiter *ratelimit.Limiter, policies Policies, totpKey []byte) *Service {
	if sessions == nil {
		panic("twofactor: session service is required")
	}
	if settingsService == nil {
		panic("twofactor: settings service is required")
	}
	return &Service{
		repo:     newRepository(db),
		sessions: sessions,
		settings: settingsService,
		limiter:  limiter,
		policies: policies,
		totpKey:  totpKey,
	}
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
	codes, err := GenerateRecoveryCodes(RecoveryCodesPerBatch)
	if err != nil {
		return nil, nil, err
	}

	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		hashes = append(hashes, HashRecoveryCode(code))
	}
	return codes, hashes, nil
}

func (s *Service) twoFactorAvailable(ctx context.Context) (bool, error) {
	enabled, err := s.settings.GetBool(ctx, "auth", "totp_enabled")
	if err != nil {
		return false, err
	}
	return enabled, nil
}

func (s *Service) isFreshSetup(row *ent.User, now time.Time) bool {
	return row.TotpSecret != nil && row.TotpSetupAt != nil && now.Sub(*row.TotpSetupAt) < SetupReuseWindow
}

func (s *Service) Setup(ctx context.Context, userID, password string) (TwoFactorSetupResponse, error) {
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
	if s.isFreshSetup(found, now) {
		return s.reuseSetup(found.Email, *found.TotpSecret)
	}

	setup, err := generateTOTPSetup(found.Email)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}
	sealed, err := SealSecret(s.totpKey, setup.Secret)
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
		if s.isFreshSetup(current, now) {
			return s.reuseSetup(current.Email, *current.TotpSecret)
		}
	}
	return TwoFactorSetupResponse{}, apierror.ErrInternal
}

func (s *Service) reuseSetup(accountName, sealed string) (TwoFactorSetupResponse, error) {
	plain, err := OpenSecret(s.totpKey, sealed)
	if err != nil {
		return TwoFactorSetupResponse{}, apierror.ErrInternal
	}
	setup, err := rebuildTOTPSetup(accountName, plain)
	if err != nil {
		return TwoFactorSetupResponse{}, apierror.ErrInternal
	}
	return TwoFactorSetupResponse{Secret: setup.Secret, OTPAuthURL: setup.OTPAuthURL}, nil
}

func (s *Service) Enable(ctx context.Context, userID, code, password, currentToken string) (RecoveryCodesResponse, error) {
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
	currentVersion := s.sessions.AuthVersion(ctx)
	if err := verifyPassword(found, password); err != nil {
		return RecoveryCodesResponse{}, err
	}
	if found.TotpSecret == nil {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorNotSetup
	}
	if found.TotpEnabledAt != nil {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorAlreadyEnabled
	}

	secret, err := OpenSecret(s.totpKey, *found.TotpSecret)
	if err != nil {
		return RecoveryCodesResponse{}, apierror.ErrInternal
	}

	now := totpNow()
	counter, matched, err := MatchTOTPCounter(code, secret, now)
	if err != nil {
		return RecoveryCodesResponse{}, apierror.ErrInternal
	}
	if !matched {
		return RecoveryCodesResponse{}, apierror.ErrTwoFactorInvalidCode
	}

	if err := s.sessions.RevokeOtherSessions(ctx, userID, currentToken); err != nil {
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
		s.sessions.SetAuthVersion(ctx, newVersion)
	}
	return RecoveryCodesResponse{RecoveryCodes: codes}, nil
}

func (s *Service) Disable(ctx context.Context, userID, password, currentToken string) error {
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

	if err := s.sessions.RevokeOtherSessions(ctx, userID, currentToken); err != nil {
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

func (s *Service) VerifyRecoveryCodes(ctx context.Context, userID string, codes []string) error {
	if len(codes) == 0 {
		return apierror.WithParam(apierror.ErrInvalidRequest, "codes")
	}

	found, err := s.activeUser(ctx, userID)
	if err != nil {
		return err
	}
	if found.TotpEnabledAt == nil {
		return apierror.ErrTwoFactorNotEnabled
	}

	hashes, err := s.repo.unusedRecoveryCodeHashes(ctx, found.ID)
	if err != nil {
		return err
	}
	allowed := make(map[string]struct{}, len(hashes))
	for _, hash := range hashes {
		allowed[hash] = struct{}{}
	}

	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		trimmed := strings.TrimSpace(code)
		if trimmed == "" || len(trimmed) > maxRecoveryCodeInputLength {
			return apierror.WithParam(apierror.ErrInvalidRequest, "codes")
		}
		hash := HashRecoveryCode(trimmed)
		if _, ok := allowed[hash]; !ok {
			return apierror.WithParam(apierror.ErrInvalidRequest, "codes")
		}
		if _, duplicate := seen[hash]; duplicate {
			return apierror.WithParam(apierror.ErrInvalidRequest, "codes")
		}
		seen[hash] = struct{}{}
	}
	return nil
}

func (s *Service) VerifySecondFactor(ctx context.Context, code string, meta session.Meta) (User, error) {
	if err := s.allowIP(meta.IP); err != nil {
		return User{}, err
	}

	pendingUserID := s.sessions.PendingUserID(ctx)
	if pendingUserID == "" {
		return User{}, apierror.ErrUnauthorized
	}

	key := accountKey(pendingUserID)
	if err := s.checkAccountFailure(key); err != nil {
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
		consumed, err := s.repo.consumeRecoveryCode(ctx, found.ID, HashRecoveryCode(code))
		if err != nil {
			return User{}, err
		}
		verified = consumed
	}
	if !verified {
		s.recordAccountFailure(key)
		return User{}, s.failSecondFactor(ctx)
	}
	s.resetAccountFailure(key)

	if err := s.sessions.Establish(ctx, found.ID.String(), found.AuthVersion, meta); err != nil {
		return User{}, err
	}

	return User{Email: found.Email, Nickname: found.Nickname, Bio: found.Bio}, nil
}

func (s *Service) matchTOTP(ctx context.Context, found *ent.User, code string) (bool, error) {
	if found.TotpSecret == nil {
		return false, apierror.ErrInternal
	}

	secret, err := OpenSecret(s.totpKey, *found.TotpSecret)
	if err != nil {
		return false, apierror.ErrInternal
	}

	counter, matched, err := MatchTOTPCounter(code, secret, totpNow())
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
	attempts := s.sessions.PendingAttempts(ctx) + 1
	if attempts >= maxVerifyAttempts {
		s.sessions.Destroy(ctx)
		return apierror.ErrTwoFactorLocked
	}

	s.sessions.SetPendingAttempts(ctx, attempts)
	return apierror.ErrTwoFactorInvalidCode
}
