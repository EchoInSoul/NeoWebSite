package auth

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"website/backend/internal/config"
	"website/backend/internal/database"
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

	resp := post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password})
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
	setup := setup2FA(t, client, ts.URL, password)

	if err := svc.SetBool(context.Background(), "auth", "totp_enabled", false); err != nil {
		t.Fatalf("disable totp: %v", err)
	}

	resp := post2FA(t, client, ts.URL, "enable", TwoFactorEnableRequest{Code: codeAt(t, setup.Secret, currentWindow()), Password: password})
	if resp.StatusCode != http.StatusForbidden {
		resp.Body.Close()
		t.Fatalf("enable status = %d, want 403", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "two_factor_disabled" {
		t.Fatalf("enable code = %q, want two_factor_disabled", code)
	}
}

func TestLoginStillRequiresSecondFactorWhenSiteSwitchOff(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetTOTPEnabled(t, svc)

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)
	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)

	if err := svc.SetBool(context.Background(), "auth", "totp_enabled", false); err != nil {
		t.Fatalf("disable totp: %v", err)
	}

	assertErrorCode(t, loginRaw(t, testClient(t), ts.URL, user.Email, password), http.StatusUnauthorized, "two_factor_required")
}

func TestRecoveryCodesStillWorkWhenSiteSwitchOff(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetTOTPEnabled(t, svc)

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)
	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)

	if err := svc.SetBool(context.Background(), "auth", "totp_enabled", false); err != nil {
		t.Fatalf("disable totp: %v", err)
	}

	resp := post2FA(t, client, ts.URL, "recovery-codes", TwoFactorDisableRequest{Password: password})
	codes := decodeJSON[RecoveryCodesResponse](t, resp, http.StatusOK)
	if len(codes.RecoveryCodes) != recoveryCodesPerBatch {
		t.Fatalf("recovery codes = %d, want %d", len(codes.RecoveryCodes), recoveryCodesPerBatch)
	}
}

func TestSecondFactorVerifyAppliesConfiguredSessionMaxAge(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, sm, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetSessionMaxAge(t, svc)
	resetTOTPEnabled(t, svc)

	ctx := context.Background()
	if err := svc.SetBool(ctx, "auth", "session_max_age_enabled", true); err != nil {
		t.Fatalf("enable session max age: %v", err)
	}
	if err := svc.SetNumber(ctx, "auth", "session_max_age_days", 3); err != nil {
		t.Fatalf("set session max age days: %v", err)
	}

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)
	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)
	logoutOK(t, client, ts.URL)

	pending := testClient(t)
	assertErrorCode(t, loginRaw(t, pending, ts.URL, user.Email, password), http.StatusUnauthorized, "two_factor_required")
	decodeJSON[UserResponse](t, verify2FA(t, pending, ts.URL, nextWindowCode(t, setup.Secret)), http.StatusOK)

	_, deadline, _ := sessionRow(t, sm, ts.URL, pending)
	want := 3 * 24 * time.Hour
	remaining := time.Until(deadline)
	if remaining < want-time.Minute || remaining > want+time.Minute {
		t.Fatalf("session lifetime after verify = %v, want about %v", remaining, want)
	}
}

func brokenSettingsService(t *testing.T) *settings.Service {
	t.Helper()

	broken, err := database.Open(context.Background(), config.Config{
		DatabaseURL:       os.Getenv("DATABASE_URL_TEST"),
		DBMaxOpenConns:    1,
		DBMaxIdleConns:    1,
		DBConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("open broken db: %v", err)
	}
	module, err := settings.New(broken)
	if err != nil {
		_ = broken.Close()
		t.Fatalf("settings.New on broken db: %v", err)
	}
	if err := broken.Close(); err != nil {
		t.Fatalf("close broken db: %v", err)
	}
	return module.Service
}

func TestLoginFailsWhenSettingsUnavailable(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, _ := newTestAppWithSessionsAndSettings(t, db, brokenSettingsService(t))
	user, password := createTestUser(t, db, tag)

	client := testClient(t)
	assertErrorCode(t, loginRaw(t, client, ts.URL, user.Email, password), http.StatusInternalServerError, "internal_error")
}

func TestTOTPSetupFailsWhenSettingsUnavailable(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)

	if _, err := db.SQL().Exec("DELETE FROM setting_values WHERE setting_group = $1 AND name = $2", "auth", "totp_enabled"); err != nil {
		t.Fatalf("delete auth.totp_enabled: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.ResetItem(context.Background(), "auth", "totp_enabled")
	})

	resp := post2FA(t, client, ts.URL, "setup", TwoFactorSetupRequest{Password: password})
	assertErrorCode(t, resp, http.StatusInternalServerError, "internal_error")
}

func TestDisableTOTPStillWorksWhenGloballyDisabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, svc := newTestAppWithSessionsAndSettings(t, db)
	user, password := createTestUser(t, db, tag)
	resetTOTPEnabled(t, svc)

	client := testClient(t)
	loginOK(t, client, ts.URL, user.Email, password)
	setup := setup2FA(t, client, ts.URL, password)
	enable2FA(t, client, ts.URL, setup.Secret, password)

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
