package passwordreset

import (
	"context"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/apierror"
	pwreset "website/backend/internal/database/ent/passwordreset"
)

func requestResetToken(t *testing.T, service *Service, email string) string {
	t.Helper()

	mailer := service.mailer.(*fakeMailer)
	before := len(mailer.sent())
	if err := service.Request(context.Background(), email, "203.0.130.1"); err != nil {
		t.Fatalf("Request: %v", err)
	}
	messages := mailer.sent()
	if len(messages) != before+1 {
		t.Fatalf("sent messages = %d, want %d", len(messages), before+1)
	}
	return extractResetToken(t, messages[len(messages)-1].Body.Text)
}

func TestConfirmChangesPasswordAndVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUserWithPassword(t, db, tag, "Initial123")
	token := requestResetToken(t, service, user.Email)

	if err := service.Confirm(ctx, token, "NewPass123", "203.0.130.2"); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	stored, err := db.Client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("NewPass123")) != nil {
		t.Fatal("password was not changed to the submitted one")
	}
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("Initial123")) == nil {
		t.Fatal("old password still verifies")
	}
	if stored.AuthVersion != user.AuthVersion+1 {
		t.Fatalf("auth version = %d, want %d", stored.AuthVersion, user.AuthVersion+1)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("reset rows after confirm = %d, want 0", count)
	}
}

func TestConfirmSingleUse(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	token := requestResetToken(t, service, user.Email)

	if err := service.Confirm(ctx, token, "NewPass123", "203.0.131.1"); err != nil {
		t.Fatalf("first Confirm: %v", err)
	}
	err := service.Confirm(ctx, token, "OtherPass123", "203.0.131.1")
	if err != apierror.ErrPasswordResetInvalid {
		t.Fatalf("second Confirm error = %v, want ErrPasswordResetInvalid", err)
	}

	stored, err := db.Client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("NewPass123")) != nil {
		t.Fatal("the replayed confirmation changed the password")
	}
}

func TestConfirmExpiredDeletesRow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	if _, err := db.Client.PasswordReset.Create().
		SetUserID(user.ID).
		SetTokenHash(hashToken("expired")).
		SetExpiresAt(time.Now().Add(-time.Minute)).
		Save(ctx); err != nil {
		t.Fatalf("seed expired reset: %v", err)
	}

	if err := service.Confirm(ctx, "expired", "NewPass123", "203.0.132.1"); err != apierror.ErrPasswordResetInvalid {
		t.Fatalf("Confirm error = %v, want ErrPasswordResetInvalid", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("expired reset rows = %d, want 0", count)
	}
}

func TestConfirmUnknownToken(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	if err := service.Confirm(ctx, "never-issued", "NewPass123", "203.0.133.1"); err != apierror.ErrPasswordResetInvalid {
		t.Fatalf("Confirm error = %v, want ErrPasswordResetInvalid", err)
	}
}

func TestConfirmWeakPasswordKeepsToken(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	token := requestResetToken(t, service, user.Email)

	if err := service.Confirm(ctx, token, "abc", "203.0.134.1"); err == nil {
		t.Fatal("Confirm with a weak password returned nil")
	}
	rows, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).All(ctx)
	if err != nil {
		t.Fatalf("query resets: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("reset rows after a weak password = %d, want 1", len(rows))
	}

	if err := service.Confirm(ctx, token, "NewPass123", "203.0.134.1"); err != nil {
		t.Fatalf("Confirm with an acceptable password: %v", err)
	}
}

func TestConfirmSamePasswordKeepsToken(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUserWithPassword(t, db, tag, "Initial123")
	token := requestResetToken(t, service, user.Email)

	err := service.Confirm(ctx, token, "Initial123", "203.0.135.1")
	if err != apierror.ErrPasswordUnchanged {
		t.Fatalf("Confirm error = %v, want ErrPasswordUnchanged", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 1 {
		t.Fatalf("reset rows = %d, want 1", count)
	}

	if err := service.Confirm(ctx, token, "NewPass123", "203.0.135.1"); err != nil {
		t.Fatalf("Confirm after the unchanged-password rejection: %v", err)
	}
}

func TestConfirmDisabled(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	token := requestResetToken(t, service, user.Email)

	if err := settingsService.SetBool(ctx, "auth", "password_reset_enabled", false); err != nil {
		t.Fatalf("disable password reset: %v", err)
	}
	if err := service.Confirm(ctx, token, "NewPass123", "203.0.136.1"); err != apierror.ErrPasswordResetDisabled {
		t.Fatalf("Confirm error = %v, want ErrPasswordResetDisabled", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 1 {
		t.Fatalf("reset rows while disabled = %d, want 1", count)
	}
}

func TestConfirmRateLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	for range confirmIPLimit {
		if err := service.Confirm(ctx, "never-issued", "NewPass123", "203.0.137.7"); err != apierror.ErrPasswordResetInvalid {
			t.Fatalf("Confirm inside the IP limit error = %v, want ErrPasswordResetInvalid", err)
		}
	}

	err := service.Confirm(ctx, "never-issued", "NewPass123", "203.0.137.7")
	if err == nil {
		t.Fatal("Confirm past the IP limit returned nil")
	}
	if code := apierror.Body(err).Code; code != "rate_limited" {
		t.Fatalf("code = %q, want rate_limited", code)
	}
}

func TestConfirmConcurrent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	token := requestResetToken(t, service, user.Email)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	passwords := []string{"Winner123", "Loser123"}
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs[i] = service.Confirm(ctx, token, passwords[i], "203.0.138.1")
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range errs {
		if err == nil {
			successes++
			continue
		}
		if err != apierror.ErrPasswordResetInvalid {
			t.Fatalf("concurrent Confirm error = %v, want ErrPasswordResetInvalid", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successful confirmations = %d, want exactly 1 (errs = %v)", successes, errs)
	}

	stored, err := db.Client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	matched := 0
	for _, candidate := range passwords {
		if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(candidate)) == nil {
			matched++
		}
	}
	if matched != 1 {
		t.Fatalf("stored password matches %d candidates, want exactly 1", matched)
	}
}

func TestConfirmKeepsTwoFactor(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	enabledAt := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	setupAt := time.Now().Add(-3 * time.Hour).Truncate(time.Second)
	if _, err := db.Client.User.UpdateOneID(user.ID).
		SetTotpSecret("totp-secret").
		SetTotpEnabledAt(enabledAt).
		SetTotpLastCounter(7).
		SetTotpSetupAt(setupAt).
		Save(ctx); err != nil {
		t.Fatalf("enable two factor: %v", err)
	}

	token := requestResetToken(t, service, user.Email)
	if err := service.Confirm(ctx, token, "NewPass123", "203.0.139.1"); err != nil {
		t.Fatalf("Confirm: %v", err)
	}

	stored, err := db.Client.User.Get(ctx, user.ID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if stored.TotpSecret == nil || *stored.TotpSecret != "totp-secret" {
		t.Fatalf("totp secret = %v, want it kept", stored.TotpSecret)
	}
	if stored.TotpEnabledAt == nil || !stored.TotpEnabledAt.Equal(enabledAt) {
		t.Fatalf("totp enabled at = %v, want %v", stored.TotpEnabledAt, enabledAt)
	}
	if stored.TotpLastCounter == nil || *stored.TotpLastCounter != 7 {
		t.Fatalf("totp last counter = %v, want 7", stored.TotpLastCounter)
	}
	if stored.TotpSetupAt == nil || !stored.TotpSetupAt.Equal(setupAt) {
		t.Fatalf("totp setup at = %v, want %v", stored.TotpSetupAt, setupAt)
	}
}

func TestConfirmConfiguredMinLength(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	if err := settingsService.SetBool(ctx, "auth", "password_requirements_enabled", true); err != nil {
		t.Fatalf("enable password requirements: %v", err)
	}
	if err := settingsService.SetNumber(ctx, "auth", "password_min_length", 12); err != nil {
		t.Fatalf("set password min length: %v", err)
	}
	t.Cleanup(func() {
		_ = settingsService.ResetItem(ctx, "auth", "password_requirements_enabled")
		_ = settingsService.ResetItem(ctx, "auth", "password_min_length")
	})

	user := createTestUser(t, db, tag)
	token := requestResetToken(t, service, user.Email)

	err := service.Confirm(ctx, token, "Ab12345678", "203.0.140.1")
	if err == nil {
		t.Fatal("a ten character password passed a twelve character policy")
	}
	if code := apierror.Body(err).Code; code != "weak_password" {
		t.Fatalf("code = %q, want weak_password", code)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 1 {
		t.Fatalf("reset rows after a rejected password = %d, want 1", count)
	}

	if err := service.Confirm(ctx, token, "Ab1234567890", "203.0.140.1"); err != nil {
		t.Fatalf("Confirm with a twelve character password: %v", err)
	}
}
