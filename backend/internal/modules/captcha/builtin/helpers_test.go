package builtin

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/modules/settings"
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

func mustGenerator(t *testing.T) *generator {
	t.Helper()

	generator, err := newGenerator()
	if err != nil {
		t.Fatalf("newGenerator: %v", err)
	}
	return generator
}

func newTestSettings(t *testing.T, db *database.Database) *settings.Service {
	t.Helper()

	module, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return module.Service
}

func setCaptchaSettings(t *testing.T, svc *settings.Service, enabled bool, provider, scopes string) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetBool(ctx, "captcha", "captcha_enabled", enabled); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if err := svc.SetString(ctx, "captcha", "captcha_provider", provider); err != nil {
		t.Fatalf("set provider: %v", err)
	}
	if err := svc.SetString(ctx, "captcha", "captcha_scopes", scopes); err != nil {
		t.Fatalf("set scopes: %v", err)
	}

	t.Cleanup(func() {
		_ = svc.SetBool(context.Background(), "captcha", "captcha_enabled", false)
		_ = svc.SetString(context.Background(), "captcha", "captcha_provider", "none")
		_ = svc.SetString(context.Background(), "captcha", "captcha_scopes", "login,register")
	})
}

func challengeAnswer(t *testing.T, db *database.Database, id uuid.UUID) (int, int) {
	t.Helper()

	row, err := db.Client.CaptchaChallenge.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("read challenge: %v", err)
	}
	return row.AnswerX, row.AnswerY
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

func newServiceHarness(t *testing.T, db *database.Database, policies Policies) (*Service, *settings.Service) {
	t.Helper()

	settingsSvc := newTestSettings(t, db)
	service := newService(settingsSvc, newRepository(db), mustGenerator(t), ratelimit.New(), policies)
	return service, settingsSvc
}
