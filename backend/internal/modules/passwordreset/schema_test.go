package passwordreset

import (
	"context"
	"testing"
	"time"

	"website/backend/internal/database/ent"
	pwreset "website/backend/internal/database/ent/passwordreset"
)

func TestPasswordResetSchemaRoundTrip(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	user := createTestUser(t, db, tag)
	expiresAt := time.Now().Add(30 * time.Minute)

	row, err := db.Client.PasswordReset.Create().
		SetUserID(user.ID).
		SetTokenHash("h-" + tag).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create reset: %v", err)
	}

	found, err := db.Client.PasswordReset.Get(ctx, row.ID)
	if err != nil {
		t.Fatalf("read back reset: %v", err)
	}
	if found.UserID != user.ID {
		t.Fatalf("user id = %v, want %v", found.UserID, user.ID)
	}
	if found.TokenHash != "h-"+tag {
		t.Fatalf("token hash = %q, want %q", found.TokenHash, "h-"+tag)
	}
	if !found.ExpiresAt.Round(time.Second).Equal(expiresAt.Round(time.Second)) {
		t.Fatalf("expires at = %v, want %v", found.ExpiresAt, expiresAt)
	}
	if found.CreatedAt.IsZero() {
		t.Fatal("created at is zero")
	}
}

func TestPasswordResetRejectsSecondRowPerUser(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	user := createTestUser(t, db, tag)

	if _, err := db.Client.PasswordReset.Create().
		SetUserID(user.ID).
		SetTokenHash("first-" + tag).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(ctx); err != nil {
		t.Fatalf("create first reset: %v", err)
	}

	_, err := db.Client.PasswordReset.Create().
		SetUserID(user.ID).
		SetTokenHash("second-" + tag).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(ctx)
	if !ent.IsConstraintError(err) {
		t.Fatalf("second row per user error = %v, want constraint error", err)
	}
}

func TestPasswordResetRejectsDuplicateTokenHash(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	first := createTestUser(t, db, "a"+tag)
	second := createTestUser(t, db, "b"+tag)

	if _, err := db.Client.PasswordReset.Create().
		SetUserID(first.ID).
		SetTokenHash("shared-" + tag).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(ctx); err != nil {
		t.Fatalf("create first reset: %v", err)
	}

	_, err := db.Client.PasswordReset.Create().
		SetUserID(second.ID).
		SetTokenHash("shared-" + tag).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(ctx)
	if !ent.IsConstraintError(err) {
		t.Fatalf("duplicate token hash error = %v, want constraint error", err)
	}
}

func TestPasswordResetCascadesOnUserDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	user := createTestUser(t, db, tag)

	if _, err := db.Client.PasswordReset.Create().
		SetUserID(user.ID).
		SetTokenHash("cascade-" + tag).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(ctx); err != nil {
		t.Fatalf("create reset: %v", err)
	}
	if err := db.Client.User.DeleteOneID(user.ID).Exec(ctx); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("reset rows after user delete = %d, want 0", count)
	}
}

func TestPasswordResetColumnShape(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	var nullable string
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT is_nullable FROM information_schema.columns WHERE table_name = 'password_resets' AND column_name = 'user_id'`,
	).Scan(&nullable); err != nil {
		t.Fatalf("column lookup: %v", err)
	}
	if nullable != "NO" {
		t.Fatalf("user_id is_nullable = %q, want NO", nullable)
	}
}

func TestPasswordResetIndexShape(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	var uniqueUserID int
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT count(*) FROM pg_indexes WHERE tablename = 'password_resets' AND indexdef LIKE 'CREATE UNIQUE INDEX%' AND indexdef LIKE '%(user_id)%'`,
	).Scan(&uniqueUserID); err != nil {
		t.Fatalf("unique user_id index lookup: %v", err)
	}
	if uniqueUserID != 1 {
		t.Fatalf("unique indexes on user_id = %d, want 1", uniqueUserID)
	}

	var expiresIndex int
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT count(*) FROM pg_indexes WHERE tablename = 'password_resets' AND indexdef LIKE '%(expires_at)%'`,
	).Scan(&expiresIndex); err != nil {
		t.Fatalf("expires_at index lookup: %v", err)
	}
	if expiresIndex != 1 {
		t.Fatalf("indexes on expires_at = %d, want 1", expiresIndex)
	}
}
