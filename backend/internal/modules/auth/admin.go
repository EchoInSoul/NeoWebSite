package auth

import (
	"context"
	"errors"
	"strings"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/userinput"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
)

const emailVerificationNotRequired = "not_required"

func CreateAdminUser(ctx context.Context, db *database.Database, email, nickname, password string) (User, error) {
	if len(password) > userinput.PasswordMaxBytes {
		return User{}, errors.New("auth: password must be at most 72 bytes")
	}
	if password == "" {
		return User{}, errors.New("auth: password is required")
	}

	email = strings.ToLower(strings.TrimSpace(email))
	nickname = strings.TrimSpace(nickname)
	if email == "" || nickname == "" {
		return User{}, errors.New("auth: email and nickname are required")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}

	created, err := newRepository(db).createUser(ctx, email, nickname, string(hash), emailVerificationNotRequired)
	if err != nil {
		if ent.IsConstraintError(err) {
			return User{}, errors.New("auth: email or nickname already exists")
		}
		return User{}, err
	}

	return toUser(created), nil
}
