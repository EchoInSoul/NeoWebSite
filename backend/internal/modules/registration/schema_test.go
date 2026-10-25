package registration

import (
	"context"
	"testing"
	"time"

	"website/backend/internal/database/ent/user"
)

func TestPendingRegistrationSchema(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	row, err := db.Client.PendingRegistration.Create().
		SetEmail("p+" + tag + "@test.local").
		SetNickname("p" + tag).
		SetPasswordHash("hash").
		SetTokenHash("token-" + tag).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create pending: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.PendingRegistration.DeleteOneID(row.ID).Exec(context.Background()) })

	if row.Email == "" || row.PasswordHash != "hash" {
		t.Fatalf("roundtrip = %+v", row)
	}
}

func TestUserRequiresEmailVerificationStatus(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	_, err := db.Client.User.Create().
		SetEmail("nostatus+" + tag + "@test.local").
		SetNickname("ns" + tag).
		SetPasswordHash("hash").
		Save(context.Background())
	if err == nil {
		t.Fatal("user without email_verification_status saved, want validation error")
	}
}

func TestUserRejectsUnknownEmailVerificationStatus(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	_, err := db.Client.User.Create().
		SetEmail("badstatus+" + tag + "@test.local").
		SetNickname("bs" + tag).
		SetPasswordHash("hash").
		SetEmailVerificationStatus("pending").
		Save(context.Background())
	if err == nil {
		t.Fatal("unknown email_verification_status saved, want validation error")
	}
}

func TestEmailVerificationStatusColumnShape(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	var nullable string
	if err := db.SQL().QueryRowContext(ctx,
		`SELECT is_nullable FROM information_schema.columns WHERE table_name = 'users' AND column_name = 'email_verification_status'`,
	).Scan(&nullable); err != nil {
		t.Fatalf("column lookup: %v", err)
	}
	if nullable != "NO" {
		t.Fatalf("is_nullable = %q, want NO", nullable)
	}
	empty, err := db.Client.User.Query().Where(user.EmailVerificationStatusEQ("")).Count(ctx)
	if err != nil {
		t.Fatalf("count empty status: %v", err)
	}
	if empty != 0 {
		t.Fatalf("users with empty status = %d, want 0", empty)
	}
}
