package builtin

import (
	"context"
	"testing"
	"time"

	"website/backend/internal/database/ent"
)

func TestCaptchaChallengeSchema(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created, err := db.Client.CaptchaChallenge.Create().
		SetScope("login").
		SetAnswerX(12).
		SetAnswerY(34).
		SetExpiresAt(time.Now().Add(3 * time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create challenge: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(created.ID).Exec(context.Background()) })

	if created.Status != statusPending {
		t.Fatalf("status = %d, want %d", created.Status, statusPending)
	}
	if created.TokenHash != nil {
		t.Fatalf("token_hash = %v, want nil", created.TokenHash)
	}
	if created.CreatedAt.IsZero() {
		t.Fatal("created_at not set")
	}

	hash := "h" + uniqueTag(t)
	row, err := db.Client.CaptchaChallenge.Create().
		SetScope("login").SetAnswerX(1).SetAnswerY(2).
		SetTokenHash(hash).SetExpiresAt(time.Now().Add(time.Minute)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create with token hash: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(row.ID).Exec(context.Background()) })

	if _, err := db.Client.CaptchaChallenge.Create().
		SetScope("login").SetAnswerX(1).SetAnswerY(2).
		SetTokenHash(hash).SetExpiresAt(time.Now().Add(time.Minute)).
		Save(ctx); !ent.IsConstraintError(err) {
		t.Fatalf("duplicate token_hash err = %v, want constraint error", err)
	}
}
