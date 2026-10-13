package auth

import (
	"context"
	"errors"
	"time"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/settings"
)

const (
	pendingUserIDKey   = "auth.pending_2fa_user_id"
	pendingAttemptsKey = "auth.pending_2fa_attempts"
	maxVerifyAttempts  = 5
)

const recoveryCodesPerBatch = 10

type Service struct {
	repo      *repository
	sessions  *scs.SessionManager
	dummyHash []byte
	totpKey   []byte
	settings  *settings.Service
}

func newService(db *database.Database, sessions *scs.SessionManager, totpKey []byte, settingsService *settings.Service) *Service {
	dummy, err := bcrypt.GenerateFromPassword([]byte("timing-defense-placeholder"), bcrypt.DefaultCost)
	if err != nil {
		dummy = nil
	}
	return &Service{
		repo:      newRepository(db),
		sessions:  sessions,
		dummyHash: dummy,
		totpKey:   totpKey,
		settings:  settingsService,
	}
}

func (s *Service) Register(ctx context.Context, email, nickname, password string, meta SessionMeta) (User, error) {
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
	if !validPassword(password) {
		return User{}, apierror.ErrWeakPassword
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

	if err := s.establishSession(ctx, created.ID.String(), meta); err != nil {
		return User{}, err
	}

	return toUser(created), nil
}

func (s *Service) Login(ctx context.Context, identifier, password string, meta SessionMeta) (User, error) {
	if len(password) > maxPasswordBytes {
		return User{}, apierror.ErrInvalidCredentials
	}

	found, err := s.repo.findUserByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
			return User{}, apierror.ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		return User{}, apierror.ErrInvalidCredentials
	}

	if found.TotpEnabledAt != nil {
		if err := s.sessions.RenewToken(ctx); err != nil {
			return User{}, err
		}
		s.enterPendingSession(ctx, found.ID.String())
		return User{}, apierror.ErrTwoFactorRequired
	}

	if err := s.establishSession(ctx, found.ID.String(), meta); err != nil {
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

	if !validPassword(newPassword) {
		return apierror.ErrWeakPassword
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(currentPassword)); err != nil {
		return apierror.ErrInvalidCredentials
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

	return s.repo.updatePasswordHash(ctx, userID, string(hash))
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

func (s *Service) UpdateProfile(ctx context.Context, userID string, nickname, email *string) (User, error) {
	if nickname == nil && email == nil {
		return User{}, apierror.ErrInvalidRequest
	}

	var nextNickname, nextEmail *string

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

	updated, err := s.repo.updateProfile(ctx, userID, nextNickname, nextEmail)
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

func (s *Service) SetupTOTP(ctx context.Context, userID string) (TwoFactorSetupResponse, error) {
	found, err := s.activeUser(ctx, userID)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}
	if found.TotpEnabledAt != nil {
		return TwoFactorSetupResponse{}, apierror.ErrTwoFactorAlreadyEnabled
	}

	setup, err := generateTOTPSetup(found.Email)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}
	sealed, err := sealSecret(s.totpKey, setup.Secret)
	if err != nil {
		return TwoFactorSetupResponse{}, err
	}
	if err := s.repo.updateTOTPSetup(ctx, found.ID, sealed); err != nil {
		return TwoFactorSetupResponse{}, err
	}
	return TwoFactorSetupResponse{Secret: setup.Secret, OTPAuthURL: setup.OTPAuthURL}, nil
}

func (s *Service) EnableTOTP(ctx context.Context, userID, code, currentToken string) (RecoveryCodesResponse, error) {
	found, err := s.activeUser(ctx, userID)
	if err != nil {
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
	if err := s.repo.enableTOTP(ctx, found.ID, now, hashes); err != nil {
		return RecoveryCodesResponse{}, err
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
	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		return apierror.ErrInvalidCredentials
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
	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		return RecoveryCodesResponse{}, apierror.ErrInvalidCredentials
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
	pendingUserID := s.sessions.GetString(ctx, pendingUserIDKey)
	if pendingUserID == "" {
		return User{}, apierror.ErrUnauthorized
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
		return User{}, s.failSecondFactor(ctx)
	}

	if err := s.establishSession(ctx, found.ID.String(), meta); err != nil {
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
