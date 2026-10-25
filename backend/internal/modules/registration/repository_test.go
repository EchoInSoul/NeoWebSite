package registration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/pendingregistration"
	"website/backend/internal/database/ent/user"
)

func testDB(t *testing.T) *database.Database {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}

	db, err := database.Open(context.Background(), config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func uniqueTag(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func pendingCount(t *testing.T, db *database.Database, email string) int {
	t.Helper()
	count, err := db.Client.PendingRegistration.Query().
		Where(pendingregistration.EmailEQ(email)).
		Count(context.Background())
	if err != nil {
		t.Fatalf("count pending: %v", err)
	}
	return count
}

func TestRegisterCompensationDoesNotDeleteReplacedPending(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	repo := newRepository(db)
	email := "replace+" + tag + "@example.com"
	first, err := repo.replacePendingRegistration(ctx, pendingRegistrationInput{
		Email: email, Nickname: "r" + tag, PasswordHash: "hash-a", TokenHash: "token-a-" + tag,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("first replace: %v", err)
	}
	second, err := repo.replacePendingRegistration(ctx, pendingRegistrationInput{
		Email: email, Nickname: "r" + tag, PasswordHash: "hash-b", TokenHash: "token-b-" + tag,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	})
	if err != nil {
		t.Fatalf("second replace: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.PendingRegistration.Delete().Where(pendingregistration.EmailEQ(email)).Exec(context.Background())
	})

	if _, err := repo.deletePendingRegistration(ctx, first.ID, first.TokenHash); err != nil {
		t.Fatalf("compensate first: %v", err)
	}
	count := pendingCount(t, db, email)
	if count != 1 {
		t.Fatalf("pending = %d, want the replaced record to survive", count)
	}
	row, err := db.Client.PendingRegistration.Query().Where(pendingregistration.EmailEQ(email)).Only(ctx)
	if err != nil {
		t.Fatalf("find pending: %v", err)
	}
	if row.TokenHash != second.TokenHash {
		t.Fatalf("token hash = %q, want second %q", row.TokenHash, second.TokenHash)
	}
}

func TestVerifyDeleteCountGuard(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	email := "guard+" + tag + "@example.com"
	_, hash, err := newPendingToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	repo := newRepository(db)
	if _, err := repo.replacePendingRegistration(ctx, pendingRegistrationInput{
		Email: email, Nickname: "gd" + tag, PasswordHash: "hash", TokenHash: hash,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.PendingRegistration.Delete().
			Where(pendingregistration.EmailEQ(email)).Exec(context.Background())
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background())
	})
	row, err := db.Client.PendingRegistration.Query().
		Where(pendingregistration.EmailEQ(email)).Only(ctx)
	if err != nil {
		t.Fatalf("find pending: %v", err)
	}

	sqlTx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin tx: %v", err)
	}
	defer func() { _ = sqlTx.Rollback() }()
	var locked string
	if err := sqlTx.QueryRowContext(ctx,
		`SELECT id::text FROM pending_registrations WHERE id = $1 FOR UPDATE`,
		row.ID.String(),
	).Scan(&locked); err != nil {
		t.Fatalf("lock pending: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := newRepository(db).verifyPendingRegistration(context.Background(), hash)
		result <- err
	}()
	time.Sleep(500 * time.Millisecond)
	if _, err := sqlTx.ExecContext(ctx,
		`DELETE FROM pending_registrations WHERE id = $1`, row.ID.String(),
	); err != nil {
		t.Fatalf("delete locked pending: %v", err)
	}
	if err := sqlTx.Commit(); err != nil {
		t.Fatalf("commit delete: %v", err)
	}

	select {
	case err := <-result:
		if !errors.Is(err, errPendingNotFound) {
			t.Fatalf("verify error = %v, want errPendingNotFound (delete count guard)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("verify did not finish")
	}
}

func TestVerifyStaleTokenDoesNotConsumeReplacedPending(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	email := "stale+" + tag + "@example.com"
	oldPassword, err := bcrypt.GenerateFromPassword([]byte("oldpass123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("old password hash: %v", err)
	}
	newPassword, err := bcrypt.GenerateFromPassword([]byte("newpass123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("new password hash: %v", err)
	}
	_, oldHash, err := newPendingToken()
	if err != nil {
		t.Fatalf("old token: %v", err)
	}
	_, newHash, err := newPendingToken()
	if err != nil {
		t.Fatalf("new token: %v", err)
	}
	repo := newRepository(db)
	if _, err := repo.replacePendingRegistration(ctx, pendingRegistrationInput{
		Email:        email,
		Nickname:     "old" + tag,
		PasswordHash: string(oldPassword),
		TokenHash:    oldHash,
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.PendingRegistration.Delete().
			Where(pendingregistration.EmailEQ(email)).Exec(context.Background())
		_, _ = db.Client.User.Delete().
			Where(user.EmailEQ(email)).Exec(context.Background())
	})

	lockTx, err := db.SQL().BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin lock tx: %v", err)
	}
	defer func() { _ = lockTx.Rollback() }()
	if _, err := lockTx.ExecContext(ctx, `LOCK TABLE users IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatalf("lock users: %v", err)
	}

	result := make(chan error, 1)
	go func() {
		_, err := newRepository(db).verifyPendingRegistration(context.Background(), oldHash)
		result <- err
	}()
	waitForUsersLockWaiter(t, db)

	if _, err := repo.replacePendingRegistration(ctx, pendingRegistrationInput{
		Email:        email,
		Nickname:     "new" + tag,
		PasswordHash: string(newPassword),
		TokenHash:    newHash,
		ExpiresAt:    time.Now().Add(30 * time.Minute),
	}); err != nil {
		t.Fatalf("replace pending: %v", err)
	}
	if err := lockTx.Commit(); err != nil {
		t.Fatalf("release users lock: %v", err)
	}

	select {
	case err := <-result:
		if !errors.Is(err, errPendingNotFound) {
			t.Fatalf("stale verify error = %v, want errPendingNotFound", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stale verify did not finish")
	}

	row, err := db.Client.PendingRegistration.Query().
		Where(pendingregistration.EmailEQ(email)).Only(ctx)
	if err != nil {
		t.Fatalf("find pending after stale verify: %v", err)
	}
	if row.TokenHash != newHash {
		t.Fatalf("pending token hash = %q, want the resubmitted token", row.TokenHash)
	}

	created, err := repo.verifyPendingRegistration(ctx, newHash)
	if err != nil {
		t.Fatalf("verify resubmitted token: %v", err)
	}
	if created.Nickname != "new"+tag {
		t.Fatalf("created nickname = %q, want the resubmitted nickname", created.Nickname)
	}
	if bcrypt.CompareHashAndPassword([]byte(created.PasswordHash), []byte("newpass123")) != nil {
		t.Fatal("created user does not carry the resubmitted password")
	}
}

func waitForUsersLockWaiter(t *testing.T, db *database.Database) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var waiters int
		if err := db.SQL().QueryRowContext(context.Background(),
			`SELECT count(*) FROM pg_locks WHERE relation = 'public.users'::regclass AND NOT granted`,
		).Scan(&waiters); err != nil {
			t.Fatalf("count users lock waiters: %v", err)
		}
		if waiters > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("verification did not block on the users table lock")
}
