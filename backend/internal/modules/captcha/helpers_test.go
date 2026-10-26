package captcha

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/database"
	"website/backend/internal/modules/captcha/builtin"
	"website/backend/internal/modules/settings"
)

func setCaptchaSettings(t *testing.T, svc *settings.Service, enabled bool, provider string) {
	t.Helper()

	setCaptchaMode(t, svc, "slide")
	ctx := context.Background()
	if err := svc.SetBool(ctx, "captcha", "captcha_enabled", enabled); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if err := svc.SetString(ctx, "captcha", "captcha_provider", provider); err != nil {
		t.Fatalf("set provider: %v", err)
	}

	t.Cleanup(func() {
		_ = svc.SetBool(context.Background(), "captcha", "captcha_enabled", false)
		_ = svc.SetString(context.Background(), "captcha", "captcha_provider", "none")
		_ = svc.SetString(context.Background(), "captcha", "captcha_mode", "slide")
	})
}

func setCaptchaMode(t *testing.T, svc *settings.Service, mode string) {
	t.Helper()

	if err := svc.SetString(context.Background(), "captcha", "captcha_mode", mode); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.SetString(context.Background(), "captcha", "captcha_mode", "slide")
	})
}

func challengeDots(t *testing.T, db *database.Database, id uuid.UUID) []builtin.AnswerDot {
	t.Helper()

	row, err := db.Client.CaptchaChallenge.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	var payload builtin.AnswerDots
	if err := json.Unmarshal(row.AnswerDots, &payload); err != nil {
		t.Fatalf("decode answer dots: %v", err)
	}
	return payload.Dots
}

func challengePromptText(t *testing.T, db *database.Database, id uuid.UUID) string {
	t.Helper()

	row, err := db.Client.CaptchaChallenge.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	return row.PromptText
}

func deleteChallenge(t *testing.T, db *database.Database, id uuid.UUID) {
	t.Helper()

	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(id).Exec(context.Background()) })
}

func assertErrorCode(t *testing.T, err error, code string) {
	t.Helper()

	if err == nil {
		t.Fatalf("err = nil, want code %q", code)
	}
	if got := apierror.Body(err).Code; got != code {
		t.Fatalf("code = %q, want %q (err: %v)", got, code, err)
	}
}
