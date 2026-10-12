package auth

import (
	"context"
	"net/http"
	"testing"

	"website/backend/internal/database"
	"website/backend/internal/modules/settings"
)

func closeRegistration(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetBool(ctx, "register", "enabled", false); err != nil {
		t.Fatalf("set register.enabled = false: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "register", "enabled"); err != nil {
			t.Fatalf("reset register.enabled: %v", err)
		}
	})
}

func userCount(t *testing.T, db *database.Database) int {
	t.Helper()

	count, err := db.Client.User.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	return count
}

func TestRegisterBlockedWhenSettingOff(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	closeRegistration(t, settingsService)

	email := "closed+" + tag + "@example.com"
	cleanupUserByEmail(t, db, email)
	before := userCount(t, db)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/register", RegisterRequest{Email: email, Nickname: "closed" + tag, Password: "pass1234"})
	status := resp.StatusCode
	code := decodeError(t, resp)
	if status != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", status)
	}
	if code != "registration_closed" {
		t.Fatalf("code = %q, want registration_closed", code)
	}
	if after := userCount(t, db); after != before {
		t.Fatalf("users = %d after blocked register, want %d", after, before)
	}
}

func TestRegisterGatePrecedesFieldValidation(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	closeRegistration(t, settingsService)

	body := RegisterRequest{Email: "no-at", Nickname: "nickgate", Password: "pass1234"}

	closed := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/register", body)
	if code := decodeError(t, closed); code != "registration_closed" {
		t.Fatalf("closed: code = %q, want registration_closed", code)
	}

	if err := settingsService.SetBool(ctx, "register", "enabled", true); err != nil {
		t.Fatalf("reopen register.enabled: %v", err)
	}
	opened := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/register", body)
	if code := decodeError(t, opened); code != "invalid_request" {
		t.Fatalf("open: code = %q, want invalid_request", code)
	}
}

func TestLoginUnaffectedWhenRegistrationClosed(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)

	email := "login+" + tag + "@example.com"
	cleanupUserByEmail(t, db, email)
	created := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/register", RegisterRequest{Email: email, Nickname: "login" + tag, Password: "pass1234"})
	if created.StatusCode != http.StatusCreated {
		created.Body.Close()
		t.Fatalf("setup register status = %d, want 201", created.StatusCode)
	}
	created.Body.Close()

	closeRegistration(t, settingsService)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: email, Password: "pass1234"})
	if resp.StatusCode != http.StatusOK {
		code := decodeError(t, resp)
		t.Fatalf("login while registration closed: status = %d, code = %q, want 200", resp.StatusCode, code)
	}
	resp.Body.Close()
}

func TestCreateAdminUserUnaffectedWhenClosed(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	_, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	closeRegistration(t, settingsService)

	email := "admin+" + tag + "@example.com"
	cleanupUserByEmail(t, db, email)

	user, err := CreateAdminUser(context.Background(), db, email, "adm"+tag, "pass1234")
	if err != nil {
		t.Fatalf("CreateAdminUser while closed = %v, want success", err)
	}
	if user.Email != email {
		t.Fatalf("email = %q, want %q", user.Email, email)
	}
}
