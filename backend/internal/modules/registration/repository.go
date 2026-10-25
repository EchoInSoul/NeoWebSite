package registration

import (
	"context"
	"errors"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/pendingregistration"
	"website/backend/internal/database/ent/user"
)

var (
	errUserNotFound         = errors.New("registration: user not found")
	errPendingNotFound      = errors.New("registration: pending registration not found")
	errPendingEmailTaken    = errors.New("registration: pending email taken")
	errPendingNicknameTaken = errors.New("registration: pending nickname taken")
	errPendingConflict      = errors.New("registration: pending registration conflict")
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

func (r *repository) replacePendingRegistration(ctx context.Context, in pendingRegistrationInput) (pendingRegistrationRecord, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return pendingRegistrationRecord{}, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.PendingRegistration.Delete().
		Where(pendingregistration.ExpiresAtLT(time.Now())).
		Exec(ctx); err != nil {
		return pendingRegistrationRecord{}, err
	}

	if err := tx.PendingRegistration.Create().
		SetEmail(in.Email).
		SetNickname(in.Nickname).
		SetPasswordHash(in.PasswordHash).
		SetTokenHash(in.TokenHash).
		SetExpiresAt(in.ExpiresAt).
		OnConflict(sql.ConflictColumns("email")).
		Update(func(u *ent.PendingRegistrationUpsert) {
			u.SetNickname(in.Nickname)
			u.SetPasswordHash(in.PasswordHash)
			u.SetTokenHash(in.TokenHash)
			u.SetExpiresAt(in.ExpiresAt)
		}).
		Exec(ctx); err != nil {
		return pendingRegistrationRecord{}, err
	}

	row, err := tx.PendingRegistration.Query().
		Where(pendingregistration.EmailEQ(in.Email)).
		Only(ctx)
	if err != nil {
		return pendingRegistrationRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return pendingRegistrationRecord{}, err
	}
	return pendingRegistrationRecord{ID: row.ID, TokenHash: row.TokenHash}, nil
}

func (r *repository) deletePendingRegistration(ctx context.Context, id uuid.UUID, tokenHash string) (int, error) {
	return r.client.PendingRegistration.Delete().
		Where(pendingregistration.IDEQ(id), pendingregistration.TokenHashEQ(tokenHash)).
		Exec(ctx)
}

func (r *repository) verifyPendingRegistration(ctx context.Context, tokenHash string) (*ent.User, error) {
	row, err := r.client.PendingRegistration.Query().
		Where(pendingregistration.TokenHashEQ(tokenHash)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, errPendingNotFound
		}
		return nil, err
	}
	if !row.ExpiresAt.After(time.Now()) {
		if _, err := r.client.PendingRegistration.Delete().
			Where(pendingregistration.IDEQ(row.ID), pendingregistration.TokenHashEQ(tokenHash)).
			Exec(ctx); err != nil {
			return nil, err
		}
		return nil, errPendingNotFound
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.User.Query().Where(user.EmailEQ(row.Email)).Only(ctx); err == nil {
		if _, delErr := tx.PendingRegistration.Delete().
			Where(pendingregistration.IDEQ(row.ID), pendingregistration.TokenHashEQ(tokenHash)).
			Exec(ctx); delErr != nil {
			return nil, delErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, commitErr
		}
		return nil, errPendingEmailTaken
	} else if !ent.IsNotFound(err) {
		return nil, err
	}

	if _, err := tx.User.Query().Where(user.NicknameEQ(row.Nickname)).Only(ctx); err == nil {
		if _, delErr := tx.PendingRegistration.Delete().
			Where(pendingregistration.IDEQ(row.ID), pendingregistration.TokenHashEQ(tokenHash)).
			Exec(ctx); delErr != nil {
			return nil, delErr
		}
		if commitErr := tx.Commit(); commitErr != nil {
			return nil, commitErr
		}
		return nil, errPendingNicknameTaken
	} else if !ent.IsNotFound(err) {
		return nil, err
	}

	deleted, err := tx.PendingRegistration.Delete().
		Where(
			pendingregistration.IDEQ(row.ID),
			pendingregistration.TokenHashEQ(tokenHash),
			pendingregistration.ExpiresAtGT(time.Now()),
		).
		Exec(ctx)
	if err != nil {
		return nil, err
	}
	if deleted != 1 {
		return nil, errPendingNotFound
	}

	created, err := tx.User.Create().
		SetEmail(row.Email).
		SetNickname(row.Nickname).
		SetPasswordHash(row.PasswordHash).
		SetEmailVerificationStatus(emailVerificationVerified).
		Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, errPendingConflict
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func toUser(u *ent.User) User {
	return User{
		Email:    u.Email,
		Nickname: u.Nickname,
		Bio:      u.Bio,
	}
}
