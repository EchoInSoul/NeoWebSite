package auth

import (
	"context"
	"net/http"
	"testing"

	"website/backend/internal/modules/settings"
)

func enableDomainControl(t *testing.T, svc *settings.Service, domains string) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetString(ctx, "register", "allowed_email_domains", domains); err != nil {
		t.Fatalf("set allowed_email_domains: %v", err)
	}
	if err := svc.SetBool(ctx, "register", "email_domain_control_enabled", true); err != nil {
		t.Fatalf("set email_domain_control_enabled: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "register", "email_domain_control_enabled"); err != nil {
			t.Fatalf("reset email_domain_control_enabled: %v", err)
		}
		if err := svc.ResetItem(ctx, "register", "allowed_email_domains"); err != nil {
			t.Fatalf("reset allowed_email_domains: %v", err)
		}
	})
}

func enablePasswordRequirements(t *testing.T, svc *settings.Service, minLength int64) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetNumber(ctx, "auth", "password_min_length", minLength); err != nil {
		t.Fatalf("set password_min_length: %v", err)
	}
	if err := svc.SetBool(ctx, "auth", "password_requirements_enabled", true); err != nil {
		t.Fatalf("set password_requirements_enabled: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "auth", "password_requirements_enabled"); err != nil {
			t.Fatalf("reset password_requirements_enabled: %v", err)
		}
		if err := svc.ResetItem(ctx, "auth", "password_min_length"); err != nil {
			t.Fatalf("reset password_min_length: %v", err)
		}
	})
}

func TestRegisterDomainControlBlocksOtherDomains(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	enableDomainControl(t, settingsService, "example.com")

	blocked := "blocked+" + tag + "@gmail.com"
	before := userCount(t, db)
	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: blocked, Nickname: "blk" + tag, Password: "pass1234"})
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("blocked status = %d, want 400", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "email_domain_not_allowed" {
		t.Fatalf("blocked code = %q, want email_domain_not_allowed", code)
	}
	if after := userCount(t, db); after != before {
		t.Fatalf("users = %d after blocked register, want %d", after, before)
	}

	cleanupUserByEmail(t, db, "ok+"+tag+"@example.com")
	allowed := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: "ok+" + tag + "@Example.COM", Nickname: "ok" + tag, Password: "pass1234"})
	allowed.Body.Close()
	if allowed.StatusCode != http.StatusCreated {
		t.Fatalf("allowed status = %d, want 201", allowed.StatusCode)
	}
}

func TestRegisterDomainControlEmptyListAllowsAnyDomain(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	enableDomainControl(t, settingsService, " , ，")

	emails := []string{"any1+" + tag + "@gmail.com", "any2+" + tag + "@example.org"}
	for i, email := range emails {
		nickname := "any" + tag + string(rune('a'+i))
		t.Cleanup(func() {
			cleanupUserByEmail(t, db, email)
		})
		resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: nickname, Password: "pass1234"})
		if resp.StatusCode != http.StatusCreated {
			code := decodeError(t, resp)
			t.Fatalf("%s: status = %d, want 201 (code %q)", email, resp.StatusCode, code)
		}
	}
}

func TestRegisterPasswordPolicyMinLength(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	enablePasswordRequirements(t, settingsService, 10)

	weak := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: "weak+" + tag + "@example.com", Nickname: "wk" + tag, Password: "pass12345"})
	if weak.StatusCode != http.StatusBadRequest {
		weak.Body.Close()
		t.Fatalf("weak status = %d, want 400", weak.StatusCode)
	}
	if code := decodeError(t, weak); code != "weak_password" {
		t.Fatalf("weak code = %q, want weak_password", code)
	}

	cleanupUserByEmail(t, db, "strong+"+tag+"@example.com")
	strong := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: "strong+" + tag + "@example.com", Nickname: "str" + tag, Password: "pass123456"})
	strong.Body.Close()
	if strong.StatusCode != http.StatusCreated {
		t.Fatalf("strong status = %d, want 201", strong.StatusCode)
	}
}

func TestChangePasswordFollowsRequirementsSetting(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _, settingsService := newTestAppWithSessionsAndSettings(t, db)
	enablePasswordRequirements(t, settingsService, 10)

	client := testClient(t)
	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	weak := postJSON(t, client, ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "pass12345"})
	if weak.StatusCode != http.StatusBadRequest {
		weak.Body.Close()
		t.Fatalf("weak change status = %d, want 400", weak.StatusCode)
	}
	if code := decodeError(t, weak); code != "weak_password" {
		t.Fatalf("weak change code = %q, want weak_password", code)
	}

	ok := postJSON(t, client, ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "pass123456"})
	ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("ok change status = %d, want 200", ok.StatusCode)
	}
}
