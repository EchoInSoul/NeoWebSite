package auth

import (
	"context"
	"errors"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/guard"
	"website/backend/internal/database"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUnauthorized       = errors.New("unauthorized")
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

func (s *Service) Login(ctx context.Context, identifier, password string) (User, error) {
	if len(password) > maxPasswordBytes {
		return User{}, ErrInvalidCredentials
	}

	found, err := s.repo.findUserByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
			return User{}, ErrInvalidCredentials
		}
		return User{}, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(found.PasswordHash), []byte(password)); err != nil {
		return User{}, ErrInvalidCredentials
	}

	s.sessions.RenewToken(ctx)
	s.sessions.Put(ctx, guard.SessionUserIDKey, found.ID.String())

	return toUser(found), nil
}

func (s *Service) Logout(ctx context.Context) {
	s.sessions.Destroy(ctx)
}

func (s *Service) CurrentUser(ctx context.Context, userID string) (User, error) {
	found, err := s.repo.findUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, errUserNotFound) {
			s.sessions.Destroy(ctx)
			return User{}, ErrUnauthorized
		}
		return User{}, err
	}
	return toUser(found), nil
}
