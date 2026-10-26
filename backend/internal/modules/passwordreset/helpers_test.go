package passwordreset

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/mail"
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

func createTestUserWithPassword(t *testing.T, db *database.Database, tag, password string) *ent.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	created, err := db.Client.User.Create().
		SetEmail("reset+" + tag + "@example.com").
		SetNickname("reset" + tag).
		SetPasswordHash(string(hash)).
		SetEmailVerificationStatus("not_required").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.User.DeleteOneID(created.ID).Exec(context.Background()) })
	return created
}

func createTestUser(t *testing.T, db *database.Database, tag string) *ent.User {
	t.Helper()
	return createTestUserWithPassword(t, db, tag, "Initial123")
}

type fakeMailer struct {
	mu            sync.Mutex
	configured    bool
	configuredErr error
	sendSyncErr   error
	messages      []mail.Message
}

func (f *fakeMailer) Configured(context.Context) (bool, error) {
	return f.configured, f.configuredErr
}

func (f *fakeMailer) Send(context.Context, mail.Message) error { return nil }

func (f *fakeMailer) SendSync(_ context.Context, msg mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendSyncErr != nil {
		return f.sendSyncErr
	}
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeMailer) sent() []mail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	copied := make([]mail.Message, len(f.messages))
	copy(copied, f.messages)
	return copied
}

func (f *fakeMailer) Close(context.Context) error { return nil }

func newResetService(t *testing.T, db *database.Database, mailer *fakeMailer) (*Service, *settings.Service) {
	t.Helper()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return New(db, settingsModule.Service, ratelimit.New(), mailer), settingsModule.Service
}

func enablePasswordReset(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetBool(ctx, "auth", "password_reset_enabled", true); err != nil {
		t.Fatalf("enable password reset: %v", err)
	}
	t.Cleanup(func() { _ = svc.ResetItem(ctx, "auth", "password_reset_enabled") })
}

func setSite(t *testing.T, svc *settings.Service, title, rawURL string) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetString(ctx, "site", "title", title); err != nil {
		t.Fatalf("set site.title: %v", err)
	}
	if err := svc.SetString(ctx, "site", "url", rawURL); err != nil {
		t.Fatalf("set site.url: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.ResetItem(ctx, "site", "title")
		_ = svc.ResetItem(ctx, "site", "url")
	})
}

func extractResetToken(t *testing.T, body string) string {
	t.Helper()

	match := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("no reset token in %q", body)
	}
	return match[1]
}
