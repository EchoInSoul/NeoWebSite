package auth

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"website/backend/internal/modules/settings"
)

func openRegistration(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetBool(ctx, "register", "enabled", true); err != nil {
		t.Fatalf("set register.enabled = true: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "register", "enabled"); err != nil {
			t.Fatalf("reset register.enabled: %v", err)
		}
	})
}

func TestLoginRateLimitLocksIdentifier(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _, _ := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	client := testClient(t)

	for i := 0; i < 5; i++ {
		resp := loginRaw(t, client, ts.URL, u.Email, "wrong-password-1")
		assertErrorCode(t, resp, http.StatusUnauthorized, "invalid_credentials")
	}

	blocked := loginRaw(t, client, ts.URL, u.Email, "wrong-password-2")
	assertErrorCode(t, blocked, http.StatusTooManyRequests, "rate_limited")
	retry := blocked.Header.Get("Retry-After")
	if seconds, err := strconv.Atoi(retry); err != nil || seconds < 1 {
		t.Fatalf("Retry-After = %q, want a positive integer", retry)
	}

	correct := loginRaw(t, client, ts.URL, u.Email, password)
	assertErrorCode(t, correct, http.StatusTooManyRequests, "rate_limited")
}

func TestLoginRateLimitUnknownIdentifierParity(t *testing.T) {
	db := testDB(t)
	ts, _, _ := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	client := testClient(t)
	identifier := "missing+" + uniqueTag(t) + "@example.com"

	for i := 0; i < 5; i++ {
		resp := loginRaw(t, client, ts.URL, identifier, "wrong-password-1")
		assertErrorCode(t, resp, http.StatusUnauthorized, "invalid_credentials")
	}

	blocked := loginRaw(t, client, ts.URL, identifier, "wrong-password-2")
	assertErrorCode(t, blocked, http.StatusTooManyRequests, "rate_limited")
}

func TestLoginRateLimitResetsOnSuccess(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _, _ := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	client := testClient(t)

	for i := 0; i < 4; i++ {
		resp := loginRaw(t, client, ts.URL, u.Email, "wrong-password-1")
		assertErrorCode(t, resp, http.StatusUnauthorized, "invalid_credentials")
	}

	loginOK(t, client, ts.URL, u.Email, password)

	for i := 0; i < 4; i++ {
		resp := loginRaw(t, client, ts.URL, u.Email, "wrong-password-2")
		assertErrorCode(t, resp, http.StatusUnauthorized, "invalid_credentials")
	}

	fifth := loginRaw(t, client, ts.URL, u.Email, "wrong-password-3")
	assertErrorCode(t, fifth, http.StatusUnauthorized, "invalid_credentials")

	sixth := loginRaw(t, client, ts.URL, u.Email, "wrong-password-4")
	assertErrorCode(t, sixth, http.StatusTooManyRequests, "rate_limited")
}

func TestLoginRateLimitIPWindow(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	policies := disabledRateLimitPolicies()
	policies.LoginIP = windowPolicy{Limit: 2, Window: time.Hour}
	ts, _, _ := newTestAppWithRatePolicies(t, db, policies)
	client := testClient(t)

	for i := 0; i < 2; i++ {
		identifier := "missing-" + strconv.Itoa(i) + "+" + tag + "@example.com"
		resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: identifier, Password: "wrong-password-1"}, nil)
		assertErrorCode(t, resp, http.StatusUnauthorized, "invalid_credentials")
	}

	third := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: "missing-2+" + tag + "@example.com", Password: "wrong-password-1"}, nil)
	assertErrorCode(t, third, http.StatusTooManyRequests, "rate_limited")

	other := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: "missing-3+" + tag + "@example.com", Password: "wrong-password-1"}, map[string]string{
		"X-Forwarded-For": "203.0.113.8",
	})
	assertErrorCode(t, other, http.StatusUnauthorized, "invalid_credentials")
}

func TestRegisterRateLimitIP(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	openRegistration(t, settingsService)
	client := testClient(t)

	for i := 0; i < 5; i++ {
		email := "limit-ip-" + strconv.Itoa(i) + "+" + tag + "@example.com"
		resp := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "ip" + strconv.Itoa(i) + tag, Password: "short"})
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("attempt %d status = %d, want 400", i+1, resp.StatusCode)
		}
		resp.Body.Close()
	}

	sixth := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: "limit-ip-5+" + tag + "@example.com", Nickname: "ip5" + tag, Password: "short"})
	assertErrorCode(t, sixth, http.StatusTooManyRequests, "rate_limited")

	other := postJSONWithHeaders(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: "limit-ip-other+" + tag + "@example.com", Nickname: "ipother" + tag, Password: "short"}, map[string]string{
		"X-Forwarded-For": "203.0.113.8",
	})
	if other.StatusCode != http.StatusBadRequest {
		other.Body.Close()
		t.Fatalf("other ip status = %d, want 400", other.StatusCode)
	}
	other.Body.Close()
}

func TestRegisterRateLimitEmail(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts, _, settingsService := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	openRegistration(t, settingsService)
	client := testClient(t)
	email := "limit-email+" + tag + "@example.com"

	for i := 0; i < 3; i++ {
		resp := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "em" + strconv.Itoa(i) + tag, Password: "short"})
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("attempt %d status = %d, want 400", i+1, resp.StatusCode)
		}
		resp.Body.Close()
	}

	fourth := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "em3" + tag, Password: "short"})
	assertErrorCode(t, fourth, http.StatusTooManyRequests, "rate_limited")
}

func TestLoginRateLimitCooldownExpires(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	policies := disabledRateLimitPolicies()
	policies.LoginIdentifier = failurePolicy{Limit: 2, Window: time.Minute, Cooldown: 50 * time.Millisecond}
	ts, _, _ := newTestAppWithRatePolicies(t, db, policies)
	client := testClient(t)

	for i := 0; i < 2; i++ {
		resp := loginRaw(t, client, ts.URL, u.Email, "wrong-password-1")
		assertErrorCode(t, resp, http.StatusUnauthorized, "invalid_credentials")
	}

	blocked := loginRaw(t, client, ts.URL, u.Email, "wrong-password-2")
	assertErrorCode(t, blocked, http.StatusTooManyRequests, "rate_limited")

	time.Sleep(80 * time.Millisecond)
	loginOK(t, client, ts.URL, u.Email, password)
}

func enableRateLimitTOTP(t *testing.T, tsURL, email, password string) TwoFactorSetupResponse {
	t.Helper()

	client := testClient(t)
	loginOK(t, client, tsURL, email, password)
	setup := setup2FA(t, client, tsURL, password)
	enable2FA(t, client, tsURL, setup.Secret, password)
	logoutOK(t, client, tsURL)
	return setup
}

func TestVerifyRateLimitAcrossSessions(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _, _ := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	setup := enableRateLimitTOTP(t, ts.URL, u.Email, password)
	wrong := wrongCodesFor(t, setup.Secret, 10)

	for session := 0; session < 2; session++ {
		client := testClient(t)
		loginRaw(t, client, ts.URL, u.Email, password).Body.Close()
		for i := 0; i < 5; i++ {
			resp := verify2FA(t, client, ts.URL, wrong[session*5+i])
			if i < 4 {
				assertErrorCode(t, resp, http.StatusUnauthorized, "two_factor_invalid_code")
			} else {
				assertErrorCode(t, resp, http.StatusUnauthorized, "two_factor_locked")
			}
		}
	}

	client := testClient(t)
	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()
	blocked := verify2FA(t, client, ts.URL, nextWindowCode(t, setup.Secret))
	assertErrorCode(t, blocked, http.StatusTooManyRequests, "rate_limited")
}

func TestVerifyRateLimitIPGate(t *testing.T) {
	db := testDB(t)
	policies := disabledRateLimitPolicies()
	policies.VerifyIP = windowPolicy{Limit: 2, Window: time.Hour}
	ts, _, _ := newTestAppWithRatePolicies(t, db, policies)
	client := testClient(t)

	for i := 0; i < 2; i++ {
		resp := verify2FA(t, client, ts.URL, "123456")
		assertErrorCode(t, resp, http.StatusUnauthorized, "unauthorized")
	}

	third := verify2FA(t, client, ts.URL, "123456")
	assertErrorCode(t, third, http.StatusTooManyRequests, "rate_limited")
}

func TestVerifyRateLimitResetsOnSuccess(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _, _ := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	setup := enableRateLimitTOTP(t, ts.URL, u.Email, password)
	client := testClient(t)
	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()

	wrong := wrongCodesFor(t, setup.Secret, 5)
	for i := 0; i < 4; i++ {
		resp := verify2FA(t, client, ts.URL, wrong[i])
		assertErrorCode(t, resp, http.StatusUnauthorized, "two_factor_invalid_code")
	}
	decodeJSON[UserResponse](t, verify2FA(t, client, ts.URL, nextWindowCode(t, setup.Secret)), http.StatusOK)
	logoutOK(t, client, ts.URL)

	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()
	wrong = wrongCodesFor(t, setup.Secret, 5)
	for i := 0; i < 4; i++ {
		resp := verify2FA(t, client, ts.URL, wrong[i])
		assertErrorCode(t, resp, http.StatusUnauthorized, "two_factor_invalid_code")
	}
	fifth := verify2FA(t, client, ts.URL, wrong[4])
	assertErrorCode(t, fifth, http.StatusUnauthorized, "two_factor_locked")
}

func TestVerifyRateLimitConcurrentAcrossSessions(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, _, _ := newTestAppWithRatePolicies(t, db, defaultRateLimitPolicies())
	setup := enableRateLimitTOTP(t, ts.URL, u.Email, password)
	wrong := wrongCodesFor(t, setup.Secret, 10)

	clients := make([]*http.Client, len(wrong))
	for i := range clients {
		clients[i] = testClient(t)
		resp := loginRaw(t, clients[i], ts.URL, u.Email, password)
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("login %d status = %d, want 401", i, resp.StatusCode)
		}
		resp.Body.Close()
	}

	type result struct {
		index  int
		status int
		err    error
	}
	results := make(chan result, len(wrong))
	for i := range wrong {
		go func(i int) {
			resp, err := clients[i].Post(ts.URL+"/api/v1/sessions/current/verify", "application/json", strings.NewReader(`{"code":"`+wrong[i]+`"}`))
			if err != nil {
				results <- result{index: i, err: err}
				return
			}
			defer resp.Body.Close()
			results <- result{index: i, status: resp.StatusCode}
		}(i)
	}
	for range wrong {
		res := <-results
		if res.err != nil {
			t.Fatalf("verify %d: %v", res.index, res.err)
		}
		if res.status != http.StatusUnauthorized {
			t.Fatalf("verify %d status = %d, want 401", res.index, res.status)
		}
	}

	client := testClient(t)
	loginRaw(t, client, ts.URL, u.Email, password).Body.Close()
	blocked := verify2FA(t, client, ts.URL, nextWindowCode(t, setup.Secret))
	assertErrorCode(t, blocked, http.StatusTooManyRequests, "rate_limited")
}
