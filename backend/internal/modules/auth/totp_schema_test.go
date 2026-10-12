package auth

import (
	"context"
	"testing"
	"time"

	"website/backend/internal/database/ent/recoverycode"
)

func TestTOTPSchemaReserved(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	user, err := db.Client.User.Create().
		SetEmail("totp+" + tag + "@test.local").
		SetNickname("totp" + tag).
		SetPasswordHash("hash-" + tag).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user without totp fields: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(user.ID).Exec(context.Background())
	})
	if user.TotpSecret != nil {
		t.Fatalf("totp_secret default %q, want nil", *user.TotpSecret)
	}
	if user.TotpEnabledAt != nil {
		t.Fatalf("totp_enabled_at default %v, want nil", user.TotpEnabledAt)
	}

	enabled := time.Now()
	if _, err := user.Update().SetTotpSecret("JBSWY3DPEHPK3PXP").SetTotpEnabledAt(enabled).Save(ctx); err != nil {
		t.Fatalf("set totp fields: %v", err)
	}
	fetched, err := db.Client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if fetched.TotpSecret == nil || *fetched.TotpSecret != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("totp_secret roundtrip %v, want JBSWY3DPEHPK3PXP", fetched.TotpSecret)
	}
	if fetched.TotpEnabledAt == nil {
		t.Fatal("totp_enabled_at roundtrip nil, want set")
	}

	code, err := db.Client.RecoveryCode.Create().
		SetUserID(user.ID).
		SetCodeHash("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa").
		Save(ctx)
	if err != nil {
		t.Fatalf("create recovery code: %v", err)
	}
	if code.UsedAt != nil {
		t.Fatalf("used_at default %v, want nil", code.UsedAt)
	}
	before, err := db.Client.RecoveryCode.Query().Where(recoverycode.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count recovery codes: %v", err)
	}
	if before != 1 {
		t.Fatalf("recovery code count %d, want 1", before)
	}
	if err := db.Client.User.DeleteOne(user).Exec(ctx); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	after, err := db.Client.RecoveryCode.Query().Where(recoverycode.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count recovery codes after user delete: %v", err)
	}
	if after != 0 {
		t.Fatalf("recovery code count after user delete %d, want 0 (cascade)", after)
	}
}

func TestTOTPLastCounterColumn(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	user, err := db.Client.User.Create().
		SetEmail("counter+" + tag + "@test.local").
		SetNickname("counter" + tag).
		SetPasswordHash("hash-" + tag).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user without counter: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(user.ID).Exec(context.Background())
	})
	if user.TotpLastCounter != nil {
		t.Fatalf("totp_last_counter default %d, want nil", *user.TotpLastCounter)
	}

	if _, err := user.Update().SetTotpLastCounter(42).Save(ctx); err != nil {
		t.Fatalf("set counter: %v", err)
	}
	fetched, err := db.Client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if fetched.TotpLastCounter == nil || *fetched.TotpLastCounter != 42 {
		t.Fatalf("counter roundtrip %v, want 42", fetched.TotpLastCounter)
	}

	if _, err := fetched.Update().ClearTotpLastCounter().Save(ctx); err != nil {
		t.Fatalf("clear counter: %v", err)
	}
	cleared, err := db.Client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("reload user after clear: %v", err)
	}
	if cleared.TotpLastCounter != nil {
		t.Fatalf("counter after clear %d, want nil", *cleared.TotpLastCounter)
	}
}
