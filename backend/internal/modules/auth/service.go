package auth

import (
	"context"
	"errors"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
)

type Service struct {
	repo      *repository
	sessions  *scs.SessionManager
	dummyHash []byte
}

func newService(db *database.Database, sessions *scs.SessionManager) *Service {
	dummy, err := bcrypt.GenerateFromPassword([]byte("timing-defense-placeholder"), bcrypt.DefaultCost)
	if err != nil {
		dummy = nil
	}
	return &Service{
		repo:      newRepository(db),
		sessions:  sessions,
		dummyHash: dummy,
	}
}

func (s *Service) Register(ctx context.Context, email, nickname, password string) (User, error) {
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

	if err := s.sessions.RenewToken(ctx); err != nil {
		return User{}, err
	}
	s.sessions.Put(ctx, guard.SessionUserIDKey, created.ID.String())

	return toUser(created), nil
}

func (s *Service) Login(ctx context.Context, identifier, password string) (User, error) {
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

	if err := s.sessions.RenewToken(ctx); err != nil {
		return User{}, err
	}
	s.sessions.Put(ctx, guard.SessionUserIDKey, found.ID.String())

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

	if err := s.revokeOtherSessions(ctx, userID, currentToken); err != nil {
		return err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	return s.repo.updatePasswordHash(ctx, userID, string(hash))
}

func (s *Service) revokeOtherSessions(ctx context.Context, userID, currentToken string) error {
	iterable, ok := s.sessions.Store.(scs.IterableStore)
	if !ok {
		return errors.New("session store does not support iteration")
	}

	sessions, err := iterable.All()
	if err != nil {
		return err
	}

	for token, data := range sessions {
		if token == currentToken {
			continue
		}

		_, values, err := s.sessions.Codec.Decode(data)
		if err != nil {
			continue
		}

		storedUserID, ok := values[guard.SessionUserIDKey].(string)
		if !ok || storedUserID != userID {
			continue
		}

		if err := s.sessions.Store.Delete(token); err != nil {
			return err
		}
	}

	return nil
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
