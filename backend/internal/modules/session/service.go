package session

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"sort"
	"time"

	"github.com/alexedwards/scs/v2"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/guard"
)

const (
	pendingUserIDKey   = "auth.pending_2fa_user_id"
	pendingAttemptsKey = "auth.pending_2fa_attempts"
)

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
	GetNumber(ctx context.Context, group, name string) (int64, error)
}

type Service struct {
	sessions *scs.SessionManager
	settings Settings
}

func New(settings Settings, sessions *scs.SessionManager) *Service {
	return &Service{sessions: sessions, settings: settings}
}

func (s *Service) Establish(ctx context.Context, userID string, authVersion int, meta Meta) error {
	deadline, err := s.deadline(ctx)
	if err != nil {
		return err
	}

	if err := s.sessions.RenewToken(ctx); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.sessions.Put(ctx, guard.SessionUserIDKey, userID)
	s.sessions.Put(ctx, guard.SessionAuthVersionKey, authVersion)
	s.sessions.Put(ctx, guard.SessionUserAgentKey, meta.UserAgent)
	s.sessions.Put(ctx, guard.SessionIPKey, meta.IP)
	s.sessions.Put(ctx, guard.SessionCreatedAtKey, now)
	s.sessions.Put(ctx, guard.SessionLastSeenAtKey, now)
	s.sessions.Remove(ctx, pendingUserIDKey)
	s.sessions.Remove(ctx, pendingAttemptsKey)
	if deadline != nil {
		s.sessions.SetDeadline(ctx, *deadline)
	}

	return nil
}

func (s *Service) deadline(ctx context.Context) (*time.Time, error) {
	enabled, err := s.settings.GetBool(ctx, "auth", "session_max_age_enabled")
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	days, err := s.settings.GetNumber(ctx, "auth", "session_max_age_days")
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(time.Duration(days) * 24 * time.Hour)
	return &deadline, nil
}

func (s *Service) Destroy(ctx context.Context) error {
	return s.sessions.Destroy(ctx)
}

func (s *Service) EnterPending2FA(ctx context.Context, userID string) error {
	if err := s.sessions.RenewToken(ctx); err != nil {
		return err
	}
	s.sessions.Remove(ctx, guard.SessionUserIDKey)
	s.sessions.Remove(ctx, guard.SessionAuthVersionKey)
	s.sessions.Remove(ctx, guard.SessionUserAgentKey)
	s.sessions.Remove(ctx, guard.SessionIPKey)
	s.sessions.Remove(ctx, guard.SessionCreatedAtKey)
	s.sessions.Remove(ctx, guard.SessionLastSeenAtKey)
	s.sessions.Put(ctx, pendingUserIDKey, userID)
	return nil
}

func (s *Service) PendingUserID(ctx context.Context) string {
	return s.sessions.GetString(ctx, pendingUserIDKey)
}

func (s *Service) PendingAttempts(ctx context.Context) int {
	return s.sessions.GetInt(ctx, pendingAttemptsKey)
}

func (s *Service) SetPendingAttempts(ctx context.Context, attempts int) {
	s.sessions.Put(ctx, pendingAttemptsKey, attempts)
}

func (s *Service) AuthVersion(ctx context.Context) int {
	return s.sessions.GetInt(ctx, guard.SessionAuthVersionKey)
}

func (s *Service) SetAuthVersion(ctx context.Context, version int) {
	s.sessions.Put(ctx, guard.SessionAuthVersionKey, version)
}

func (s *Service) RevokeOtherSessions(ctx context.Context, userID, currentToken string) error {
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

func sessionFingerprint(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func sessionString(values map[string]any, key string) string {
	text, _ := values[key].(string)
	return text
}

func (s *Service) eachSession(fn func(token string, values map[string]any, deadline time.Time) error) error {
	iterable, ok := s.sessions.Store.(scs.IterableStore)
	if !ok {
		return errors.New("session: session store does not support iteration")
	}

	all, err := iterable.All()
	if err != nil {
		return err
	}

	for token, data := range all {
		deadline, values, err := s.sessions.Codec.Decode(data)
		if err != nil {
			continue
		}
		if err := fn(token, values, deadline); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) List(ctx context.Context, userID string) ([]Session, error) {
	currentID := sessionFingerprint(s.sessions.Token(ctx))
	sessions := make([]Session, 0)

	err := s.eachSession(func(token string, values map[string]any, deadline time.Time) error {
		if sessionString(values, guard.SessionUserIDKey) != userID {
			return nil
		}

		id := sessionFingerprint(token)
		sessions = append(sessions, Session{
			ID:         id,
			Current:    id == currentID,
			UserAgent:  sessionString(values, guard.SessionUserAgentKey),
			IP:         sessionString(values, guard.SessionIPKey),
			CreatedAt:  sessionString(values, guard.SessionCreatedAtKey),
			LastSeenAt: sessionString(values, guard.SessionLastSeenAtKey),
			ExpiresAt:  deadline.UTC().Format(time.RFC3339),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].LastSeenAt != sessions[j].LastSeenAt {
			return sessions[i].LastSeenAt > sessions[j].LastSeenAt
		}
		return sessions[i].ID < sessions[j].ID
	})

	return sessions, nil
}

var errSessionFound = errors.New("session: session found")

func (s *Service) Revoke(ctx context.Context, userID, sessionID string) error {
	var target string
	err := s.eachSession(func(token string, values map[string]any, _ time.Time) error {
		if sessionString(values, guard.SessionUserIDKey) != userID {
			return nil
		}
		if sessionFingerprint(token) != sessionID {
			return nil
		}
		target = token
		return errSessionFound
	})
	if err != nil && !errors.Is(err, errSessionFound) {
		return err
	}
	if target == "" {
		return apierror.ErrNotFound
	}

	if target == s.sessions.Token(ctx) {
		return s.sessions.Destroy(ctx)
	}
	return s.sessions.Store.Delete(target)
}
