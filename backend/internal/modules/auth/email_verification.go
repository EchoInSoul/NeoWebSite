package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

const (
	emailVerificationVerified    = "verified"
	emailVerificationNotRequired = "not_required"
	pendingRegistrationTTL       = 30 * time.Minute
	pendingTokenBytes            = 32
)

type pendingRegistrationInput struct {
	Email        string
	Nickname     string
	PasswordHash string
	TokenHash    string
	ExpiresAt    time.Time
}

type pendingRegistrationRecord struct {
	ID        uuid.UUID
	TokenHash string
}

func newPendingToken() (string, string, error) {
	buf := make([]byte, pendingTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, hashPendingToken(raw), nil
}

func hashPendingToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
