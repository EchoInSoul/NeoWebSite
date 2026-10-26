package passwordreset

import (
	"context"
	"errors"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	pwreset "website/backend/internal/database/ent/passwordreset"
	"website/backend/internal/database/ent/user"
)

var (
	errUserNotFound  = errors.New("passwordreset: user not found")
	errResetNotFound = errors.New("passwordreset: reset not found")
)

type repository struct {
	client *ent.Client
}

func newRepository(db *database.Database) *repository {
	return &repository{client: db.Client}
}

func (r *repository) findUserByEmail(ctx context.Context, email string) (*ent.User, error) {
	found, err := r.client.User.Query().Where(user.EmailEQ(email)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, errUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (r *repository) findUserByID(ctx context.Context, id uuid.UUID) (*ent.User, error) {
	found, err := r.client.User.Get(ctx, id)
	if ent.IsNotFound(err) {
		return nil, errUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (r *repository) replaceReset(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (uuid.UUID, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.PasswordReset.Delete().Where(pwreset.ExpiresAtLT(time.Now())).Exec(ctx); err != nil {
		return uuid.Nil, err
	}
	if err := tx.PasswordReset.Create().
		SetUserID(userID).
		SetTokenHash(tokenHash).
		SetExpiresAt(expiresAt).
		OnConflict(sql.ConflictColumns("user_id")).
		Update(func(u *ent.PasswordResetUpsert) {
			u.SetTokenHash(tokenHash)
			u.SetExpiresAt(expiresAt)
		}).
		Exec(ctx); err != nil {
		return uuid.Nil, err
	}

	row, err := tx.PasswordReset.Query().Where(pwreset.UserIDEQ(userID)).Only(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	if err := tx.Commit(); err != nil {
		return uuid.Nil, err
	}
	return row.ID, nil
}

func (r *repository) findReset(ctx context.Context, tokenHash string) (*ent.PasswordReset, error) {
	found, err := r.client.PasswordReset.Query().Where(pwreset.TokenHashEQ(tokenHash)).Only(ctx)
	if ent.IsNotFound(err) {
		return nil, errResetNotFound
	}
	if err != nil {
		return nil, err
	}
	return found, nil
}

func (r *repository) deleteReset(ctx context.Context, id uuid.UUID, tokenHash string) (int, error) {
	deleted, err := r.client.PasswordReset.Delete().Where(pwreset.IDEQ(id), pwreset.TokenHashEQ(tokenHash)).Exec(ctx)
	if err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *repository) completeReset(ctx context.Context, id uuid.UUID, tokenHash string, userID uuid.UUID, passwordHash string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	claimed, err := tx.PasswordReset.Delete().Where(pwreset.IDEQ(id), pwreset.TokenHashEQ(tokenHash)).Exec(ctx)
	if err != nil {
		return err
	}
	if claimed != 1 {
		return errResetNotFound
	}

	if _, err := tx.User.UpdateOneID(userID).
		SetPasswordHash(passwordHash).
		AddAuthVersion(1).
		Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return errUserNotFound
		}
		return err
	}
	return tx.Commit()
}
