package passwordreset

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"website/backend/internal/common/apierror"
	pwreset "website/backend/internal/database/ent/passwordreset"
)

func TestRequestSendsLink(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	before := time.Now()

	if err := service.Request(ctx, user.Email, "203.0.113.10"); err != nil {
		t.Fatalf("Request: %v", err)
	}

	messages := mailer.sent()
	if len(messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(messages))
	}
	msg := messages[0]
	if msg.To != user.Email {
		t.Fatalf("To = %q, want %q", msg.To, user.Email)
	}
	if !msg.RequireSMTP {
		t.Fatal("RequireSMTP = false, want true")
	}
	link := "https://blog.example.com/reset-password/confirm?token="
	if !strings.Contains(msg.Body.Text, link) || !strings.Contains(msg.Body.HTML, link) {
		t.Fatalf("body missing reset link: text=%q html=%q", msg.Body.Text, msg.Body.HTML)
	}

	rows, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).All(ctx)
	if err != nil {
		t.Fatalf("query resets: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("reset rows = %d, want 1", len(rows))
	}

	raw := extractResetToken(t, msg.Body.Text)
	if rows[0].TokenHash != hashToken(raw) {
		t.Fatalf("stored hash = %q, want hash of the emailed token", rows[0].TokenHash)
	}
	lower := before.Add(29 * time.Minute)
	upper := before.Add(31 * time.Minute)
	if rows[0].ExpiresAt.Before(lower) || rows[0].ExpiresAt.After(upper) {
		t.Fatalf("expires at = %v, want between %v and %v", rows[0].ExpiresAt, lower, upper)
	}
}

func TestRequestUnknownEmailNoSend(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	if err := service.Request(ctx, "nobody-"+tag+"@example.com", "203.0.113.11"); err != nil {
		t.Fatalf("Request with unknown email: %v", err)
	}
	if messages := mailer.sent(); len(messages) != 0 {
		t.Fatalf("sent messages = %d, want 0", len(messages))
	}

	count, err := db.Client.PasswordReset.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("reset rows = %d, want 0", count)
	}
}

func TestRequestUnknownEmailStillCountsRateLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	email := "typo-" + tag + "@example.com"
	for i := range 3 {
		ip := "10.0.0." + string(rune('1'+i))
		if err := service.Request(ctx, email, ip); err != nil {
			t.Fatalf("Request %d: %v", i+1, err)
		}
	}

	if err := service.Request(ctx, email, "10.0.0.4"); err == nil {
		t.Fatal("fourth request for an unknown email passed, want the email bucket to count")
	}
}

func TestRequestDisabled(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	err := service.Request(ctx, user.Email, "203.0.113.12")
	if err != apierror.ErrPasswordResetDisabled {
		t.Fatalf("Request error = %v, want ErrPasswordResetDisabled", err)
	}
	if messages := mailer.sent(); len(messages) != 0 {
		t.Fatalf("sent messages = %d, want 0", len(messages))
	}
}

func TestRequestInvalidEmail(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)

	for _, email := range []string{"", "not-an-email"} {
		err := service.Request(ctx, email, "203.0.113.13")
		if err == nil {
			t.Fatalf("Request(%q) returned nil, want invalid_request", email)
		}
		if body := apierror.Body(err); body.Code != "invalid_request" || body.Param == nil || *body.Param != "email" {
			t.Fatalf("Request(%q) error = %+v, want invalid_request param email", email, body)
		}
	}
	if messages := mailer.sent(); len(messages) != 0 {
		t.Fatalf("sent messages = %d, want 0", len(messages))
	}
}

func TestRequestMailUnavailable(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: false}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	if err := service.Request(ctx, user.Email, "203.0.113.14"); err != apierror.ErrMailUnavailable {
		t.Fatalf("Request error = %v, want ErrMailUnavailable", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("reset rows = %d, want 0", count)
	}
}

func TestRequestMailUnavailableBeforeLookup(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: false}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	err := service.Request(ctx, "nobody-"+tag+"@example.com", "203.0.113.15")
	if err != apierror.ErrMailUnavailable {
		t.Fatalf("Request with unknown email error = %v, want ErrMailUnavailable before the lookup", err)
	}
}

func TestRequestSiteURLInvalid(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)

	for i, raw := range []string{"", "example.com", "https://example.com/?a=1", "https://example.com/#x"} {
		mailer := &fakeMailer{configured: true}
		service, settingsService := newResetService(t, db, mailer)
		enablePasswordReset(t, settingsService)
		setSite(t, settingsService, "My Blog", raw)

		user := createTestUser(t, db, fmt.Sprintf("%s-case%d", tag, i))
		err := service.Request(ctx, user.Email, "203.0.113.16")
		if err != apierror.ErrMailUnavailable {
			t.Fatalf("Request with site.url %q error = %v, want ErrMailUnavailable", raw, err)
		}
	}

	count, err := db.Client.PasswordReset.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("reset rows = %d, want 0", count)
	}
}

func TestRequestSendFailureCompensates(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true, sendSyncErr: errors.New("smtp rejected")}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	if err := service.Request(ctx, user.Email, "203.0.113.17"); err != apierror.ErrMailUnavailable {
		t.Fatalf("Request error = %v, want ErrMailUnavailable", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("reset rows after a failed send = %d, want 0", count)
	}
}

func TestRequestComposeFailureCompensates(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	if err := settingsService.SetString(ctx, "mail", "password_reset_body", "{{.broken"); err != nil {
		t.Fatalf("set broken template: %v", err)
	}
	t.Cleanup(func() { _ = settingsService.ResetItem(ctx, "mail", "password_reset_body") })

	user := createTestUser(t, db, tag)
	if err := service.Request(ctx, user.Email, "203.0.113.18"); err != apierror.ErrMailUnavailable {
		t.Fatalf("Request error = %v, want ErrMailUnavailable", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("reset rows after a compose failure = %d, want 0", count)
	}
	if messages := mailer.sent(); len(messages) != 0 {
		t.Fatalf("sent messages = %d, want 0", len(messages))
	}
}

func TestRequestReplacesToken(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	if err := service.Request(ctx, user.Email, "203.0.113.19"); err != nil {
		t.Fatalf("first Request: %v", err)
	}
	if err := service.Request(ctx, user.Email, "203.0.113.19"); err != nil {
		t.Fatalf("second Request: %v", err)
	}

	messages := mailer.sent()
	if len(messages) != 2 {
		t.Fatalf("sent messages = %d, want 2", len(messages))
	}
	first := extractResetToken(t, messages[0].Body.Text)
	second := extractResetToken(t, messages[1].Body.Text)
	if first == second {
		t.Fatal("two requests produced the same token")
	}

	rows, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(user.ID)).All(ctx)
	if err != nil {
		t.Fatalf("query resets: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("reset rows = %d, want 1", len(rows))
	}
	if rows[0].TokenHash != hashToken(second) {
		t.Fatalf("stored hash is not the newest token")
	}
	repo := newRepository(db)
	if _, err := repo.findReset(ctx, hashToken(first)); !errors.Is(err, errResetNotFound) {
		t.Fatalf("findReset(old token) error = %v, want errResetNotFound", err)
	}
}

func TestRequestEmailRateLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	for range 3 {
		if err := service.Request(ctx, user.Email, "203.0.120.1"); err != nil {
			t.Fatalf("Request inside the email limit: %v", err)
		}
	}

	err := service.Request(ctx, user.Email, "203.0.120.1")
	if err == nil {
		t.Fatal("fourth request for the same email passed, want rate_limited")
	}
	if code := apierror.Body(err).Code; code != "rate_limited" {
		t.Fatalf("code = %q, want rate_limited", code)
	}
}

func TestRequestIPRateLimit(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	for i := range 5 {
		email := "ip" + string(rune('a'+i)) + "-" + tag + "@example.com"
		if err := service.Request(ctx, email, "203.0.121.9"); err != nil {
			t.Fatalf("Request %d inside the IP limit: %v", i+1, err)
		}
	}

	err := service.Request(ctx, "sixth-"+tag+"@example.com", "203.0.121.9")
	if err == nil {
		t.Fatal("sixth request from the same IP passed, want rate_limited")
	}
	if code := apierror.Body(err).Code; code != "rate_limited" {
		t.Fatalf("code = %q, want rate_limited", code)
	}
}

func TestRequestUsesStoredEmail(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	user := createTestUser(t, db, tag)
	if err := service.Request(ctx, strings.ToUpper(user.Email), "203.0.113.20"); err != nil {
		t.Fatalf("Request: %v", err)
	}

	messages := mailer.sent()
	if len(messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(messages))
	}
	if messages[0].To != user.Email {
		t.Fatalf("To = %q, want the stored address %q", messages[0].To, user.Email)
	}
}

func TestRequestClearsExpiredRows(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	service, settingsService := newResetService(t, db, mailer)
	enablePasswordReset(t, settingsService)
	setSite(t, settingsService, "My Blog", "https://blog.example.com")

	staleA := createTestUser(t, db, "a"+tag)
	staleB := createTestUser(t, db, "b"+tag)
	target := createTestUser(t, db, "c"+tag)

	expired := time.Now().Add(-time.Hour)
	for _, pair := range []struct {
		id   uuid.UUID
		hash string
	}{
		{staleA.ID, "expired-a-" + tag},
		{staleB.ID, "expired-b-" + tag},
	} {
		if _, err := db.Client.PasswordReset.Create().
			SetUserID(pair.id).
			SetTokenHash(pair.hash).
			SetExpiresAt(expired).
			Save(ctx); err != nil {
			t.Fatalf("seed expired reset: %v", err)
		}
	}

	if err := service.Request(ctx, target.Email, "203.0.113.21"); err != nil {
		t.Fatalf("Request: %v", err)
	}

	count, err := db.Client.PasswordReset.Query().Where(pwreset.ExpiresAtLT(time.Now())).Count(ctx)
	if err != nil {
		t.Fatalf("count expired resets: %v", err)
	}
	if count != 0 {
		t.Fatalf("expired reset rows = %d, want 0", count)
	}
}
