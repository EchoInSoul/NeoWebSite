package auth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"website/backend/internal/modules/settings"
)

func resetSessionMaxAge(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	names := []string{"session_max_age_enabled", "session_max_age_days"}
	for _, name := range names {
		if err := svc.ResetItem(ctx, "auth", name); err != nil {
			t.Fatalf("reset auth.%s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for _, name := range names {
			_ = svc.ResetItem(ctx, "auth", name)
		}
	})
}

func resetTOTPEnabled(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	if err := svc.ResetItem(ctx, "auth", "totp_enabled"); err != nil {
		t.Fatalf("reset auth.totp_enabled: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.ResetItem(ctx, "auth", "totp_enabled")
	})
}

func TestLoginAppliesConfiguredSessionMaxAge(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, sm, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetSessionMaxAge(t, svc)

	ctx := context.Background()
	if err := svc.SetBool(ctx, "auth", "session_max_age_enabled", true); err != nil {
		t.Fatalf("enable session max age: %v", err)
	}
	if err := svc.SetNumber(ctx, "auth", "session_max_age_days", 14); err != nil {
		t.Fatalf("set session max age days: %v", err)
	}

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)

	_, deadline, _ := sessionRow(t, sm, ts.URL, client)
	want := 14 * 24 * time.Hour
	remaining := time.Until(deadline)
	if remaining < want-time.Minute || remaining > want+time.Minute {
		t.Fatalf("session lifetime = %v, want about %v", remaining, want)
	}
}

func TestLoginKeepsEnvironmentLifetimeWhenSessionMaxAgeDisabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, sm, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetSessionMaxAge(t, svc)

	ctx := context.Background()
	if err := svc.SetNumber(ctx, "auth", "session_max_age_days", 30); err != nil {
		t.Fatalf("seed session max age days: %v", err)
	}

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)

	_, deadline, _ := sessionRow(t, sm, ts.URL, client)
	remaining := time.Until(deadline)
	if remaining < sm.Lifetime-time.Minute || remaining > sm.Lifetime+time.Minute {
		t.Fatalf("session lifetime = %v, want about %v", remaining, sm.Lifetime)
	}
}

func TestSetupTOTPBlockedWhenGloballyDisabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetTOTPEnabled(t, svc)

	if err := svc.SetBool(context.Background(), "auth", "totp_enabled", false); err != nil {
		t.Fatalf("disable totp: %v", err)
	}

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)

	resp := postRaw(t, client, ts.URL+"/api/v1/users/me/2fa/setup", "")
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("setup status = %d, want 403", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "two_factor_disabled" {
		t.Fatalf("setup code = %q, want two_factor_disabled", code)
	}
}

func TestEnableTOTPBlockedWhenGloballyDisabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetTOTPEnabled(t, svc)

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)
	setup := setup2FA(t, client, ts.URL)

	if err := svc.SetBool(context.Background(), "auth", "totp_enabled", false); err != nil {
		t.Fatalf("disable totp: %v", err)
	}

	resp := post2FA(t, client, ts.URL, "enable", TwoFactorCodeRequest{Code: codeAt(t, setup.Secret, currentWindow())})
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("enable status = %d, want 403", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "two_factor_disabled" {
		t.Fatalf("enable code = %q, want two_factor_disabled", code)
	}
}

func TestDisableTOTPStillWorksWhenGloballyDisabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetTOTPEnabled(t, svc)

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)
	setup := setup2FA(t, client, ts.URL)
	enable2FA(t, client, ts.URL, setup.Secret)

	if err := svc.SetBool(context.Background(), "auth", "totp_enabled", false); err != nil {
		t.Fatalf("disable totp: %v", err)
	}

	resp := post2FA(t, client, ts.URL, "disable", TwoFactorDisableRequest{Password: password})
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("disable status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}
