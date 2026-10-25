package captcha_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/modules/auth"
	"website/backend/internal/modules/captcha"
	"website/backend/internal/modules/captcha/turnstile"
	"website/backend/internal/modules/settings"
)

type fakeCaptchaVerifier struct {
	outcome turnstile.Outcome
	calls   int
	secret  string
	token   string
	ip      string
}

func (f *fakeCaptchaVerifier) Verify(_ context.Context, secret, token, remoteIP string) turnstile.Outcome {
	f.calls++
	f.secret = secret
	f.token = token
	f.ip = remoteIP
	return f.outcome
}

func newCaptchaTestApp(t *testing.T, db *database.Database, verifier captcha.Verifier, policies rateLimitPolicies) (*httptest.Server, *settings.Service) {
	t.Helper()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}

	ts, _, svc := newTestAppWithCaptchaVerifier(t, db, settingsModule.Service, ratelimit.New(), policies, verifier, &fakeMailer{configured: true})
	return ts, svc
}

func resetCaptchaSettings(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	names := []string{"captcha_enabled", "captcha_provider", "turnstile_site_key", "turnstile_secret_key"}
	for _, name := range names {
		if err := svc.ResetItem(ctx, "captcha", name); err != nil {
			t.Fatalf("reset auth.%s: %v", name, err)
		}
	}
	t.Cleanup(func() {
		for _, name := range names {
			_ = svc.ResetItem(ctx, "captcha", name)
		}
	})
}

func setCaptchaProvider(t *testing.T, svc *settings.Service, provider, secret string) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetBool(ctx, "captcha", "captcha_enabled", true); err != nil {
		t.Fatalf("enable captcha: %v", err)
	}
	if err := svc.SetString(ctx, "captcha", "captcha_provider", provider); err != nil {
		t.Fatalf("set captcha provider: %v", err)
	}
	if err := svc.SetString(ctx, "captcha", "turnstile_secret_key", secret); err != nil {
		t.Fatalf("set turnstile secret: %v", err)
	}
}

func assertCaptchaFailed(t *testing.T, resp *http.Response) {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusBadRequest)
	}

	var body struct {
		Error struct {
			Code  string  `json:"code"`
			Param *string `json:"param"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Error.Code != "captcha_failed" {
		t.Fatalf("code = %q, want captcha_failed", body.Error.Code)
	}
	if body.Error.Param == nil || *body.Error.Param != "captcha_token" {
		t.Fatalf("param = %v, want captcha_token", body.Error.Param)
	}
}

func TestLoginRequiresCaptchaWhenEnabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Passed}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	user, password := createTestUser(t, db, tag)
	setCaptchaProvider(t, svc, "turnstile", "the-secret")

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password})
	assertCaptchaFailed(t, resp)
	if cookie := sessionCookie(t, client.Jar, ts.URL); cookie != "" {
		t.Fatalf("session cookie = %q, want none after a captcha rejection", cookie)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 for a blank token", verifier.calls)
	}

	ok := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password, CaptchaToken: "the-token"})
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", ok.StatusCode, http.StatusOK)
	}
	if verifier.calls != 1 || verifier.secret != "the-secret" || verifier.token != "the-token" {
		t.Fatalf("verifier saw calls=%d secret=%q token=%q", verifier.calls, verifier.secret, verifier.token)
	}
	if verifier.ip == "" {
		t.Fatal("verifier saw an empty remote ip")
	}
}

func TestLoginRejectedCaptchaBlocksLogin(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Rejected}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	user, password := createTestUser(t, db, tag)
	setCaptchaProvider(t, svc, "turnstile", "the-secret")

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password, CaptchaToken: "bad-token"})
	assertCaptchaFailed(t, resp)
	if cookie := sessionCookie(t, client.Jar, ts.URL); cookie != "" {
		t.Fatalf("session cookie = %q, want none after a rejected captcha", cookie)
	}
}

func TestLoginRejectsWhenCaptchaUnavailable(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Unavailable}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	user, password := createTestUser(t, db, tag)
	setCaptchaProvider(t, svc, "turnstile", "the-secret")

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password, CaptchaToken: "anything"})
	assertCaptchaFailed(t, resp)
	if cookie := sessionCookie(t, client.Jar, ts.URL); cookie != "" {
		t.Fatalf("session cookie = %q, want none when the verifier is unavailable", cookie)
	}
}

func TestLoginSkipsCaptchaForOtherProviders(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Rejected}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	user, password := createTestUser(t, db, tag)
	setCaptchaProvider(t, svc, "geetest", "the-secret")

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d for a non-turnstile provider", resp.StatusCode, http.StatusOK)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}

func TestLoginSkipsCaptchaWhenDisabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Rejected}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	user, password := createTestUser(t, db, tag)

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d with captcha disabled", resp.StatusCode, http.StatusOK)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}

func TestLoginSkipsCaptchaWithoutSecret(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Rejected}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	user, password := createTestUser(t, db, tag)
	setCaptchaProvider(t, svc, "turnstile", "")

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d when no secret is configured", resp.StatusCode, http.StatusOK)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}

func TestLoginCaptchaFailuresDoNotLockAccount(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Rejected}
	ts, svc := newCaptchaTestApp(t, db, verifier, defaultRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	user, password := createTestUser(t, db, tag)
	setCaptchaProvider(t, svc, "turnstile", "the-secret")

	client := testClient(t)
	for attempt := 0; attempt < defaultRateLimitPolicies().LoginIdentifier.Limit+1; attempt++ {
		resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password, CaptchaToken: "bad-token"})
		assertCaptchaFailed(t, resp)
	}

	verifier.outcome = turnstile.Passed
	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", auth.LoginRequest{Identifier: user.Email, Password: password, CaptchaToken: "good-token"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d — captcha rejections must not consume the login failure budget", resp.StatusCode, http.StatusOK)
	}
}

func TestRegisterRequiresCaptchaWhenEnabled(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Passed}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	setCaptchaProvider(t, svc, "turnstile", "the-secret")

	before := userCount(t, db)
	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/users", auth.RegisterRequest{
		Email:    "signup+" + tag + "@example.com",
		Nickname: "signup" + tag,
		Password: "pass-" + tag,
	})
	assertCaptchaFailed(t, resp)
	if after := userCount(t, db); after != before {
		t.Fatalf("user count = %d, want %d after a rejected captcha", after, before)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0 for a blank token", verifier.calls)
	}

	ok := postJSON(t, client, ts.URL+"/api/v1/users", auth.RegisterRequest{
		Email:        "signup+" + tag + "@example.com",
		Nickname:     "signup" + tag,
		Password:     "pass-" + tag,
		CaptchaToken: "the-token",
	})
	defer ok.Body.Close()
	if ok.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want %d", ok.StatusCode, http.StatusCreated)
	}
	if after := userCount(t, db); after != before+1 {
		t.Fatalf("user count = %d, want %d", after, before+1)
	}
}

func TestRegisterRejectedCaptchaCreatesNoUser(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Rejected}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	setCaptchaProvider(t, svc, "turnstile", "the-secret")

	before := userCount(t, db)
	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/users", auth.RegisterRequest{
		Email:        "blocked+" + tag + "@example.com",
		Nickname:     "blocked" + tag,
		Password:     "pass-" + tag,
		CaptchaToken: "bad-token",
	})
	assertCaptchaFailed(t, resp)
	if after := userCount(t, db); after != before {
		t.Fatalf("user count = %d, want %d after a rejected captcha", after, before)
	}
}

func TestRegisterRejectsWhenCaptchaUnavailable(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	verifier := &fakeCaptchaVerifier{outcome: turnstile.Unavailable}
	ts, svc := newCaptchaTestApp(t, db, verifier, disabledRateLimitPolicies())
	resetCaptchaSettings(t, svc)
	setCaptchaProvider(t, svc, "turnstile", "the-secret")

	before := userCount(t, db)
	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/users", auth.RegisterRequest{
		Email:        "open+" + tag + "@example.com",
		Nickname:     "open" + tag,
		Password:     "pass-" + tag,
		CaptchaToken: "anything",
	})
	assertCaptchaFailed(t, resp)
	if after := userCount(t, db); after != before {
		t.Fatalf("user count = %d, want %d when the verifier is unavailable", after, before)
	}
}
