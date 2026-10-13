package auth

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

type SessionMeta struct {
	UserAgent string
	IP        string
}

type Session struct {
	ID         string
	Current    bool
	UserAgent  string
	IP         string
	CreatedAt  string
	LastSeenAt string
	ExpiresAt  string
}

func (s *Service) establishSession(ctx context.Context, userID string, meta SessionMeta) error {
	if err := s.sessions.RenewToken(ctx); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	s.sessions.Put(ctx, guard.SessionUserIDKey, userID)
	s.sessions.Put(ctx, guard.SessionUserAgentKey, meta.UserAgent)
	s.sessions.Put(ctx, guard.SessionIPKey, meta.IP)
	s.sessions.Put(ctx, guard.SessionCreatedAtKey, now)
	s.sessions.Put(ctx, guard.SessionLastSeenAtKey, now)
	s.sessions.Remove(ctx, pendingUserIDKey)
	s.sessions.Remove(ctx, pendingAttemptsKey)

	return nil
}

func (s *Service) enterPendingSession(ctx context.Context, userID string) {
	s.sessions.Remove(ctx, guard.SessionUserIDKey)
	s.sessions.Remove(ctx, guard.SessionUserAgentKey)
	s.sessions.Remove(ctx, guard.SessionIPKey)
	s.sessions.Remove(ctx, guard.SessionCreatedAtKey)
	s.sessions.Remove(ctx, guard.SessionLastSeenAtKey)
	s.sessions.Put(ctx, pendingUserIDKey, userID)
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
		return errors.New("session store does not support iteration")
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

func (s *Service) ListSessions(ctx context.Context, userID string) ([]Session, error) {
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

var errSessionFound = errors.New("session found")

func (s *Service) RevokeSession(ctx context.Context, userID, sessionID string) error {
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
