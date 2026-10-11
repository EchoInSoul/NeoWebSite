package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/user"
)

var errUserNotFound = errors.New("user not found")

type repository struct {
	client *ent.Client
}

func newRepository(db *database.Database) *repository {
	return &repository{client: db.Client}
}

func (r *repository) findUserByIdentifier(ctx context.Context, identifier string) (*ent.User, error) {
	found, err := r.client.User.Query().
		Where(user.Or(user.EmailEQ(strings.ToLower(identifier)), user.NicknameEQ(identifier))).
		Only(ctx)
	if ent.IsNotFound(err) {
		return nil, errUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (r *repository) findUserByID(ctx context.Context, id string) (*ent.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, errUserNotFound
	}
	found, err := r.client.User.Get(ctx, parsed)
	if ent.IsNotFound(err) {
		return nil, errUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (r *repository) createUser(ctx context.Context, email, nickname, passwordHash string) (*ent.User, error) {
	return r.client.User.Create().
		SetEmail(email).
		SetNickname(nickname).
		SetPasswordHash(passwordHash).
		Save(ctx)
}

func (r *repository) updatePasswordHash(ctx context.Context, id string, passwordHash string) error {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return errUserNotFound
	}
	if err := r.client.User.UpdateOneID(parsed).SetPasswordHash(passwordHash).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return errUserNotFound
		}
		return err
	}
	return nil
}

func toUser(u *ent.User) User {
	return User{
		ID:       u.ID.String(),
		Email:    u.Email,
		Nickname: u.Nickname,
	}
}
