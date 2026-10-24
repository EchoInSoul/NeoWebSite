package auth

import (
	"context"
	"testing"
	"time"

	"website/backend/internal/database/ent/pendingregistration"
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

func TestCreateAdminUserMarksNotRequiredAndClearsPending(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	email := "adminpending+" + tag + "@example.com"
	pending, err := db.Client.PendingRegistration.Create().
		SetEmail(email).
		SetNickname("ap" + tag).
		SetPasswordHash("hash").
		SetTokenHash("token-" + tag).
		SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.PendingRegistration.DeleteOneID(pending.ID).Exec(context.Background()) })

	if _, err := CreateAdminUser(ctx, db, email, "ap"+tag, "pass1234"); err != nil {
		t.Fatalf("CreateAdminUser: %v", err)
	}
	t.Cleanup(func() { _, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background()) })

	row, err := db.Client.User.Query().Where(user.EmailEQ(email)).Only(ctx)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if row.EmailVerificationStatus != "not_required" {
		t.Fatalf("status = %q, want not_required", row.EmailVerificationStatus)
	}
	count, err := db.Client.PendingRegistration.Query().Where(pendingregistration.EmailEQ(email)).Count(ctx)
	if err != nil {
		t.Fatalf("count pending: %v", err)
	}
	if count != 0 {
		t.Fatalf("pending rows = %d, want 0", count)
	}
}
