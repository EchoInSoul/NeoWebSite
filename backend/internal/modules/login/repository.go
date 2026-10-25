package login

import (
	"context"
	"errors"
	"strings"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/pendingregistration"
	"website/backend/internal/database/ent/user"
)

var (
	errUserNotFound    = errors.New("login: user not found")
	errPendingNotFound = errors.New("login: pending registration not found")
)

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

func (r *repository) findPendingRegistrationByEmail(ctx context.Context, email string) (*ent.PendingRegistration, error) {
	row, err := r.client.PendingRegistration.Query().
		Where(pendingregistration.EmailEQ(email)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errPendingNotFound
		}
		return nil, err
	}
	return row, nil
}
