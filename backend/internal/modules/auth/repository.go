package auth

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
	"website/backend/internal/database/ent/recoverycode"
	"website/backend/internal/database/ent/user"
)

var (
	errUserNotFound         = errors.New("auth: user not found")
	errPendingNotFound      = errors.New("auth: pending registration not found")
	errPendingEmailTaken    = errors.New("auth: pending email taken")
	errPendingNicknameTaken = errors.New("auth: pending nickname taken")
	errPendingConflict      = errors.New("auth: pending registration conflict")
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
			Where(pendingregistration.IDEQ(row.ID)).
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
		if _, delErr := tx.PendingRegistration.Delete().Where(pendingregistration.IDEQ(row.ID)).Exec(ctx); delErr != nil {
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
		if _, delErr := tx.PendingRegistration.Delete().Where(pendingregistration.IDEQ(row.ID)).Exec(ctx); delErr != nil {
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
		Where(pendingregistration.IDEQ(row.ID), pendingregistration.ExpiresAtGT(time.Now())).
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

func (r *repository) claimTOTPSetup(ctx context.Context, id uuid.UUID, encryptedSecret string, now time.Time) (bool, error) {
	claimed, err := r.client.User.Update().
		Where(
			user.IDEQ(id),
			user.TotpEnabledAtIsNil(),
			user.Or(user.TotpSetupAtIsNil(), user.TotpSetupAtLTE(now.Add(-totpSetupReuseWindow))),
		).
		SetTotpSecret(encryptedSecret).
		SetTotpSetupAt(now).
		ClearTotpLastCounter().
		Save(ctx)
	if err != nil {
		return false, err
	}
	return claimed == 1, nil
}

func (r *repository) enableTOTP(ctx context.Context, id uuid.UUID, enabledAt time.Time, codeHashes []string) (int, error) {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	updated, err := tx.User.UpdateOneID(id).
		SetTotpEnabledAt(enabledAt).
		AddAuthVersion(1).
		Save(ctx)
	if err != nil {
		return 0, err
	}
	if err := insertRecoveryCodes(ctx, tx, id, codeHashes); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return updated.AuthVersion, nil
}

func (r *repository) disableTOTP(ctx context.Context, id uuid.UUID) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if err := tx.User.UpdateOneID(id).
		ClearTotpSecret().
		ClearTotpEnabledAt().
		ClearTotpLastCounter().
		ClearTotpSetupAt().
		Exec(ctx); err != nil {
		return err
	}
	if _, err := tx.RecoveryCode.Delete().Where(recoverycode.UserIDEQ(id)).Exec(ctx); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *repository) replaceRecoveryCodes(ctx context.Context, userID uuid.UUID, codeHashes []string) error {
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.RecoveryCode.Delete().Where(recoverycode.UserIDEQ(userID)).Exec(ctx); err != nil {
		return err
	}
	if err := insertRecoveryCodes(ctx, tx, userID, codeHashes); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *repository) consumeRecoveryCode(ctx context.Context, userID uuid.UUID, hash string) (bool, error) {
	updated, err := r.client.RecoveryCode.Update().
		Where(
			recoverycode.UserIDEQ(userID),
			recoverycode.CodeHashEQ(hash),
			recoverycode.UsedAtIsNil(),
		).
		SetUsedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return updated == 1, nil
}

func (r *repository) claimTOTPCounter(ctx context.Context, id uuid.UUID, counter int64) (bool, error) {
	claimed, err := r.client.User.Update().
		Where(
			user.IDEQ(id),
			user.Or(user.TotpLastCounterIsNil(), user.TotpLastCounterLT(counter)),
		).
		SetTotpLastCounter(counter).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return claimed == 1, nil
}

func insertRecoveryCodes(ctx context.Context, tx *ent.Tx, userID uuid.UUID, codeHashes []string) error {
	builders := make([]*ent.RecoveryCodeCreate, 0, len(codeHashes))
	for _, hash := range codeHashes {
		builders = append(builders, tx.RecoveryCode.Create().SetUserID(userID).SetCodeHash(hash))
	}
	_, err := tx.RecoveryCode.CreateBulk(builders...).Save(ctx)
	return err
}
