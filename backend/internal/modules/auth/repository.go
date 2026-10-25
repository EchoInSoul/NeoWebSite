package auth

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/pendingregistration"
	"website/backend/internal/database/ent/user"
)

var errUserNotFound = errors.New("auth: user not found")

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

func (r *repository) createUser(ctx context.Context, email, nickname, passwordHash, emailVerificationStatus string) (*ent.User, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.PendingRegistration.Delete().
		Where(pendingregistration.EmailEQ(email)).
		Exec(ctx); err != nil {
		return nil, err
	}

	created, err := tx.User.Create().
		SetEmail(email).
		SetNickname(nickname).
		SetPasswordHash(passwordHash).
		SetEmailVerificationStatus(emailVerificationStatus).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func (r *repository) updatePasswordHash(ctx context.Context, id string, passwordHash string) (*ent.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, errUserNotFound
	}
	updated, err := r.client.User.UpdateOneID(parsed).
		SetPasswordHash(passwordHash).
		AddAuthVersion(1).
		Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errUserNotFound
		}
		return nil, err
	}
	return updated, nil
}

func (r *repository) updateProfile(ctx context.Context, id string, nickname, email, bio *string) (*ent.User, error) {
	parsed, err := uuid.Parse(id)
	if err != nil {
		return nil, errUserNotFound
	}

	update := r.client.User.UpdateOneID(parsed)
	if nickname != nil {
		update = update.SetNickname(*nickname)
	}
	if email != nil {
		update = update.SetEmail(*email)
	}
	if bio != nil {
		if *bio == "" {
			update = update.ClearBio()
		} else {
			update = update.SetBio(*bio)
		}
	}

	updated, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errUserNotFound
		}
		return nil, err
	}
	return updated, nil
}

func toUser(u *ent.User) User {
	return User{
		ID:               u.ID.String(),
		Email:            u.Email,
		Nickname:         u.Nickname,
		Bio:              u.Bio,
		TwoFactorEnabled: u.TotpEnabledAt != nil,
	}
}
