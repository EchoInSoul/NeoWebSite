package twofactor

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/recoverycode"
	"website/backend/internal/database/ent/user"
)

var errUserNotFound = errors.New("twofactor: user not found")

type repository struct {
	client *ent.Client
}

func newRepository(db *database.Database) *repository {
	return &repository{client: db.Client}
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

func (r *repository) claimTOTPSetup(ctx context.Context, id uuid.UUID, encryptedSecret string, now time.Time) (bool, error) {
	claimed, err := r.client.User.Update().
		Where(
			user.IDEQ(id),
			user.TotpEnabledAtIsNil(),
			user.Or(user.TotpSetupAtIsNil(), user.TotpSetupAtLTE(now.Add(-SetupReuseWindow))),
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
