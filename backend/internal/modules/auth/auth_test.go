package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/user"
	"website/backend/internal/modules/access"
	"website/backend/internal/modules/captcha"
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/registration"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/settings"
)

func newTestAppWithSessions(t *testing.T, db *database.Database) (*httptest.Server, *scs.SessionManager) {
	t.Helper()

	ts, sm, _ := newTestAppWithSessionsAndSettings(t, db)
	return ts, sm
}

func newTestAppWithSessionsAndSettings(t *testing.T, db *database.Database, overrideSettings ...*settings.Service) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	var settingsService *settings.Service
	if len(overrideSettings) > 0 {
		settingsService = overrideSettings[0]
	} else {
		settingsModule, err := settings.New(db)
		if err != nil {
			t.Fatalf("settings.New: %v", err)
		}
		settingsService = settingsModule.Service
	}

	return newTestAppWithSettings(t, db, settingsService, ratelimit.New(), disabledRateLimitPolicies())
}

func newTestAppWithRatePolicies(t *testing.T, db *database.Database, policies rateLimitPolicies) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return newTestAppWithSettings(t, db, settingsModule.Service, ratelimit.New(), policies)
}

func newTestAppWithVerifyPolicies(t *testing.T, db *database.Database, policies verifyPolicies) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := engine.SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	api := engine.Group("/api/v1")

	sessionService := session.New(settingsModule.Service, sm)
	captchaService := captcha.NewService(settingsModule.Service, nil)
	service := newService(db, sessionService, testTOTPKey, settingsModule.Service, ratelimit.New(), policies)
	loginService := login.New(db, sessionService, ratelimit.New(), captchaService)
	registrationService := registration.New(db, settingsModule.Service, ratelimit.New(), &fakeMailer{configured: true}, captchaService)
	module := &Module{Service: service, controller: newController(service, loginService, registrationService)}
	module.Register(api)
	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.TrackSessionActivity(sm), guard.LoadIdentity(accessModule.Service, sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, sm, settingsModule.Service
}

func newTestAppWithSettings(t *testing.T, db *database.Database, settingsService *settings.Service, limiter *ratelimit.Limiter, policies rateLimitPolicies) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	return newTestAppWithCaptchaVerifier(t, db, settingsService, limiter, policies, nil, &fakeMailer{configured: true})
}

func newTestAppWithSettingsAndMailer(t *testing.T, db *database.Database, settingsService *settings.Service, limiter *ratelimit.Limiter, policies rateLimitPolicies, mailer mail.Mailer) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	return newTestAppWithCaptchaVerifier(t, db, settingsService, limiter, policies, nil, mailer)
}

func newTestAppWithMailer(t *testing.T, db *database.Database, mailer mail.Mailer) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return newTestAppWithSettingsAndMailer(t, db, settingsModule.Service, ratelimit.New(), disabledRateLimitPolicies(), mailer)
}

func newTestAppWithCaptchaVerifier(t *testing.T, db *database.Database, settingsService *settings.Service, limiter *ratelimit.Limiter, policies rateLimitPolicies, verifier captcha.Verifier, mailer mail.Mailer) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	if err := engine.SetTrustedProxies(nil); err != nil {
		t.Fatalf("SetTrustedProxies: %v", err)
	}
	api := engine.Group("/api/v1")

	sessionService := session.New(settingsService, sm)
	captchaService := captcha.NewService(settingsService, verifier)
	service := newService(db, sessionService, testTOTPKey, settingsService, limiter, defaultVerifyPolicies())
	loginService := login.NewWithPolicies(db, sessionService, limiter, login.Policies{
		IdentifierFailure: policies.LoginIdentifier,
		IPWindow:          policies.LoginIP,
	}, captchaService)
	registrationService := registration.NewWithPolicies(db, settingsService, limiter, registration.Policies{
		EmailWindow:             policies.RegisterEmail,
		IPWindow:                policies.RegisterIP,
		EmailVerificationWindow: policies.EmailVerificationIP,
	}, mailer, captchaService)
	module := &Module{Service: service, controller: newController(service, loginService, registrationService)}
	module.Register(api)
	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.TrackSessionActivity(sm), guard.LoadIdentity(accessModule.Service, sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, sm, settingsService
}

func newTestApp(t *testing.T, db *database.Database) *httptest.Server {
	t.Helper()

	ts, _ := newTestAppWithSessions(t, db)
	return ts
}

func testClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func postJSON(t *testing.T, client *http.Client, endpoint string, body any) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post %s: %v", endpoint, err)
	}
	return resp
}

func postRaw(t *testing.T, client *http.Client, endpoint, body string) *http.Response {
	t.Helper()

	resp, err := client.Post(endpoint, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post %s: %v", endpoint, err)
	}
	return resp
}

func postRawWithHeaders(t *testing.T, client *http.Client, endpoint, body string, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("post %s: %v", endpoint, err)
	}
	return resp
}

func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want nested object with code", body["error"])
	}
	code, _ := errObj["code"].(string)
	return code
}

func sessionCookie(t *testing.T, jar http.CookieJar, tsURL string) string {
	t.Helper()

	parsed, err := url.Parse(tsURL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	for _, c := range jar.Cookies(parsed) {
		if c.Name == "session" {
			return c.Value
		}
	}
	return ""
}

func TestLoginSucceedsAndSetsCookie(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body UserResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Email != u.Email || body.Nickname != u.Nickname {
		t.Fatalf("body = %+v, want email %s nickname %s", body, u.Email, u.Nickname)
	}
	if sessionCookie(t, client.Jar, ts.URL) == "" {
		t.Fatal("expected a session cookie to be set")
	}
}

func TestLoginRejectsWrongPasswordAndUnknownAccount(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: "definitely-wrong"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_credentials" {
		t.Fatalf("wrong password error = %q, want invalid_credentials", got)
	}

	resp = postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: "nobody+" + tag + "@example.com", Password: "whatever"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unknown account status = %d, want 401", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_credentials" {
		t.Fatalf("unknown account error = %q, want invalid_credentials", got)
	}
}

func TestLoginRejectsMalformedRequests(t *testing.T) {
	db := testDB(t)
	ts := newTestApp(t, db)
	client := testClient(t)

	cases := []struct {
		name string
		body string
	}{
		{"broken json", "{not json"},
		{"missing password", `{"identifier":"someone@example.com"}`},
		{"empty fields", `{"identifier":"","password":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := postRaw(t, client, ts.URL+"/api/v1/sessions", tc.body)
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if got := decodeError(t, resp); got != "invalid_request" {
				t.Fatalf("error = %q, want invalid_request", got)
			}
		})
	}
}

func TestLoginRejectsLongPasswordWithoutServerError(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: strings.Repeat("a", 73)})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_credentials" {
		t.Fatalf("error = %q, want invalid_credentials", got)
	}
}

func TestLoginAcceptsMixedCaseEmail(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	resp := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: strings.ToUpper(u.Email), Password: password})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestLoginRejectsNonJSONContentType(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	base, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}

	payload, err := json.Marshal(LoginRequest{Identifier: u.Email, Password: password})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	cases := []struct {
		name        string
		contentType string
	}{
		{"text plain", "text/plain"},
		{"form urlencoded", "application/x-www-form-urlencoded"},
		{"missing", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t)
			headers := map[string]string{"Origin": base.Scheme + "://" + base.Host}
			if tc.contentType != "" {
				headers["Content-Type"] = tc.contentType
			}

			resp := postRawWithHeaders(t, client, ts.URL+"/api/v1/sessions", string(payload), headers)
			assertErrorCode(t, resp, http.StatusBadRequest, "invalid_request")
			if cookie := sessionCookie(t, client.Jar, ts.URL); cookie != "" {
				t.Fatalf("session cookie set by rejected login: %q", cookie)
			}
		})
	}
}

func TestLoginChecksOrigin(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	base, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	body := LoginRequest{Identifier: u.Email, Password: password}
	sameOrigin := base.Scheme + "://" + base.Host

	t.Run("cross origin rejected", func(t *testing.T) {
		client := testClient(t)
		resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", body, map[string]string{
			"Origin": "https://evil.example",
		})
		assertErrorCode(t, resp, http.StatusBadRequest, "invalid_request")
		if cookie := sessionCookie(t, client.Jar, ts.URL); cookie != "" {
			t.Fatalf("session cookie set by cross-origin login: %q", cookie)
		}
	})

	t.Run("opaque origin rejected", func(t *testing.T) {
		client := testClient(t)
		resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", body, map[string]string{
			"Origin": "null",
		})
		assertErrorCode(t, resp, http.StatusBadRequest, "invalid_request")
	})

	t.Run("forwarded host mismatch rejected", func(t *testing.T) {
		client := testClient(t)
		resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", body, map[string]string{
			"Origin":           sameOrigin,
			"X-Forwarded-Host": "other.example",
		})
		assertErrorCode(t, resp, http.StatusBadRequest, "invalid_request")
	})

	t.Run("proxied same origin accepted", func(t *testing.T) {
		client := testClient(t)
		resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", body, map[string]string{
			"Origin":           sameOrigin,
			"X-Forwarded-Host": base.Host,
		})
		decodeJSON[UserResponse](t, resp, http.StatusOK)
	})

	t.Run("json charset accepted", func(t *testing.T) {
		client := testClient(t)
		resp := postJSONWithHeaders(t, client, ts.URL+"/api/v1/sessions", body, map[string]string{
			"Content-Type": "application/json; charset=utf-8",
		})
		decodeJSON[UserResponse](t, resp, http.StatusOK)
	})
}

func TestRegisterRejectsNonJSONContentType(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts := newTestApp(t, db)
	client := testClient(t)

	email := "csrf+" + tag + "@example.com"
	cleanupUserByEmail(t, db, email)
	payload, err := json.Marshal(RegisterRequest{Email: email, Nickname: "csrf" + tag, Password: "pass1234"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	resp := postRawWithHeaders(t, client, ts.URL+"/api/v1/users", string(payload), map[string]string{
		"Content-Type": "text/plain",
	})
	assertErrorCode(t, resp, http.StatusBadRequest, "invalid_request")

	count, err := db.Client.User.Query().Where(user.EmailEQ(email)).Count(context.Background())
	if err != nil {
		t.Fatalf("count user: %v", err)
	}
	if count != 0 {
		t.Fatalf("non-JSON register created %d user(s)", count)
	}
}

func TestLoginRenewsSessionToken(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	first := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	first.Body.Close()
	before := sessionCookie(t, client.Jar, ts.URL)

	second := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	second.Body.Close()
	after := sessionCookie(t, client.Jar, ts.URL)

	if before == "" || after == "" {
		t.Fatalf("expected session cookies, got before=%q after=%q", before, after)
	}
	if before == after {
		t.Fatal("expected RenewToken to change the session cookie value")
	}
}

func TestStaleAuthVersionSessionRejected(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	id := uuid.MustParse(u.ID)
	ts := newTestApp(t, db)
	client := testClient(t)
	loginOK(t, client, ts.URL, u.Email, password)

	if status := meStatus(t, client, ts.URL); status != http.StatusOK {
		t.Fatalf("me before bump = %d, want 200", status)
	}

	if err := db.Client.User.UpdateOneID(id).AddAuthVersion(1).Exec(context.Background()); err != nil {
		t.Fatalf("bump auth version: %v", err)
	}

	assertErrorCode(t, doGET(t, client, ts.URL+"/api/v1/users/me"), http.StatusUnauthorized, "unauthorized")
	if status := meStatus(t, client, ts.URL); status != http.StatusUnauthorized {
		t.Fatalf("me after stale session destroyed = %d, want 401", status)
	}

	fresh := testClient(t)
	loginOK(t, fresh, ts.URL, u.Email, password)
	if status := meStatus(t, fresh, ts.URL); status != http.StatusOK {
		t.Fatalf("fresh login me = %d, want 200", status)
	}
}

func TestMeRequiresSessionAndReturnsUser(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)

	anonymous := testClient(t)
	resp, err := anonymous.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("anonymous me: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	client := testClient(t)
	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()

	resp, err = client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body MeResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Email != u.Email {
		t.Fatalf("email = %q, want %q", body.Email, u.Email)
	}
	if body.Group.SystemKey == nil || *body.Group.SystemKey != "user" {
		t.Fatalf("group system key = %v, want user", body.Group.SystemKey)
	}
	if body.Permissions == nil {
		t.Fatal("permissions = null, want []")
	}
	if len(body.Permissions) != 0 {
		t.Fatalf("permissions = %v, want empty", body.Permissions)
	}
}

func TestLogoutDestroysSession(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/sessions/current", nil)
	if err != nil {
		t.Fatalf("build logout: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("logout status = %d, want 200", resp.StatusCode)
	}
	var logoutBody map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&logoutBody); err != nil {
		t.Fatalf("decode logout body: %v", err)
	}
	resp.Body.Close()
	if len(logoutBody) != 0 {
		t.Fatalf("logout body = %v, want {}", logoutBody)
	}

	me, err := client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me after logout: %v", err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want 401", me.StatusCode)
	}
}

type deleteFailStore struct {
	scs.Store
}

func (s deleteFailStore) Delete(token string) error {
	return errors.New("store delete failed")
}

func TestLoginFailsWhenSessionStoreDeleteFails(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)
	client := testClient(t)

	first := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first login status = %d, want 200", first.StatusCode)
	}

	sm.Store = deleteFailStore{sm.Store}

	second := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	defer second.Body.Close()
	if second.StatusCode != http.StatusInternalServerError {
		t.Fatalf("second login status = %d, want 500", second.StatusCode)
	}
}

func TestLogoutFailsWhenSessionStoreDeleteFails(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts, sm := newTestAppWithSessions(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	sm.Store = deleteFailStore{sm.Store}

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/sessions/current", nil)
	if err != nil {
		t.Fatalf("build logout: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("logout status = %d, want 500", resp.StatusCode)
	}
}

func cleanupUserByEmail(t *testing.T, db *database.Database, email string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background())
	})
}

func TestRegisterSucceedsWithoutSession(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts := newTestApp(t, db)
	client := testClient(t)

	email := "New+" + tag + "@Example.COM"
	cleanupUserByEmail(t, db, strings.ToLower(email))

	resp := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "new" + tag, Password: "pass1234"})
	if resp.StatusCode != http.StatusCreated {
		resp.Body.Close()
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	var body UserResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	resp.Body.Close()
	if body.Email != strings.ToLower(email) {
		t.Fatalf("email = %q, want lowercased %q", body.Email, strings.ToLower(email))
	}
	if body.Nickname != "new"+tag {
		t.Fatalf("nickname = %q, want %q", body.Nickname, "new"+tag)
	}
	if cookie := sessionCookie(t, client.Jar, ts.URL); cookie != "" {
		t.Fatalf("register handed out session cookie %q, want none", cookie)
	}

	me, err := client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	me.Body.Close()
	if me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me status after register = %d, want 401", me.StatusCode)
	}

	loggedIn := loginAs(t, ts.URL, strings.ToLower(email), "pass1234")
	after, err := loggedIn.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me after login: %v", err)
	}
	defer after.Body.Close()
	if after.StatusCode != http.StatusOK {
		t.Fatalf("me status after login = %d, want 200", after.StatusCode)
	}
	var meBody UserResponse
	if err := json.NewDecoder(after.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if meBody.Email != strings.ToLower(email) || meBody.Nickname != "new"+tag {
		t.Fatalf("me = %+v, want %s / new%s", meBody, strings.ToLower(email), tag)
	}

	paddedEmail := "padded+" + tag + "@example.com"
	cleanupUserByEmail(t, db, paddedEmail)
	padded := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: " PADDED+" + tag + "@EXAMPLE.COM ", Nickname: "  pad" + tag + "  ", Password: "pass1234"})
	paddedBody := UserResponse{}
	if padded.StatusCode != http.StatusCreated {
		padded.Body.Close()
		t.Fatalf("padded status = %d, want 201", padded.StatusCode)
	}
	if err := json.NewDecoder(padded.Body).Decode(&paddedBody); err != nil {
		t.Fatalf("decode padded: %v", err)
	}
	padded.Body.Close()
	if paddedBody.Email != paddedEmail || paddedBody.Nickname != "pad"+tag {
		t.Fatalf("padded = %+v, want %s / pad%s", paddedBody, paddedEmail, tag)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	resp := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: strings.ToUpper(u.Email), Nickname: "other" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "email_taken" {
		t.Fatalf("error = %q, want email_taken", got)
	}
}

func TestRegisterRejectsDuplicateNickname(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, _ := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	email := "dup+" + tag + "@example.com"
	cleanupUserByEmail(t, db, email)
	resp := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: u.Nickname, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "nickname_taken" {
		t.Fatalf("error = %q, want nickname_taken", got)
	}
}

func TestRegisterRejectsWeakPasswords(t *testing.T) {
	db := testDB(t)
	ts := newTestApp(t, db)

	cases := []struct {
		name     string
		password string
	}{
		{"too short", "short1"},
		{"letters only", "abcdefgh"},
		{"digits only", "12345678"},
		{"over 72 bytes", strings.Repeat("a", 72) + "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t)
			tag := uniqueTag(t)
			resp := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: "weak+" + tag + "@example.com", Nickname: "weak" + tag, Password: tc.password})
			if resp.StatusCode != http.StatusBadRequest {
				resp.Body.Close()
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if got := decodeError(t, resp); got != "weak_password" {
				t.Fatalf("error = %q, want weak_password", got)
			}
		})
	}
}

func TestRegisterRejectsInvalidFields(t *testing.T) {
	db := testDB(t)
	ts := newTestApp(t, db)

	cases := []struct {
		name     string
		email    string
		nickname string
	}{
		{"email without at", "no-at", "nick"},
		{"email too long", strings.Repeat("a", 250) + "@b.com", "nick"},
		{"nickname too short", "ok@example.com", "a"},
		{"nickname with at", "ok@example.com", "a@b"},
		{"nickname with space", "ok@example.com", "a b"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t)
			resp := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: tc.email, Nickname: tc.nickname, Password: "pass1234"})
			if resp.StatusCode != http.StatusBadRequest {
				resp.Body.Close()
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			if got := decodeError(t, resp); got != "invalid_request" {
				t.Fatalf("error = %q, want invalid_request", got)
			}
		})
	}
}

func TestRegisterRejectsMalformedBody(t *testing.T) {
	db := testDB(t)
	ts := newTestApp(t, db)
	client := testClient(t)

	resp := postRaw(t, client, ts.URL+"/api/v1/users", "{not json")
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("broken json status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("broken json error = %q, want invalid_request", got)
	}

	resp = postRaw(t, client, ts.URL+"/api/v1/users", `{"email":"someone@example.com"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing fields status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("missing fields error = %q, want invalid_request", got)
	}
}

func TestRegisterWhileLoggedInKeepsExistingSession(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts := newTestApp(t, db)

	existingEmail := "first+" + tag + "@example.com"
	cleanupUserByEmail(t, db, existingEmail)
	seed := testClient(t)
	existing := postJSON(t, seed, ts.URL+"/api/v1/users", RegisterRequest{Email: existingEmail, Nickname: "first" + tag, Password: "pass1234"})
	existing.Body.Close()
	if existing.StatusCode != http.StatusCreated {
		t.Fatalf("first status = %d, want 201", existing.StatusCode)
	}
	loggedIn := loginAs(t, ts.URL, existingEmail, "pass1234")

	client := testClient(t)
	secondEmail := "second+" + tag + "@example.com"
	cleanupUserByEmail(t, db, secondEmail)
	second := postJSON(t, client, ts.URL+"/api/v1/users", RegisterRequest{Email: secondEmail, Nickname: "second" + tag, Password: "pass1234"})
	second.Body.Close()
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("second status = %d, want 201", second.StatusCode)
	}

	me, err := loggedIn.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want 200", me.StatusCode)
	}
	var body UserResponse
	if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Email != existingEmail {
		t.Fatalf("me email = %q, want %q", body.Email, existingEmail)
	}
	if cookie := sessionCookie(t, client.Jar, ts.URL); cookie != "" {
		t.Fatalf("register handed out session cookie %q, want none", cookie)
	}
}

func TestRegisterConcurrentSameEmail(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts := newTestApp(t, db)

	email := "race+" + tag + "@example.com"
	cleanupUserByEmail(t, db, email)

	statuses := make([]int, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for i := range statuses {
		go func(index int) {
			defer waitGroup.Done()
			resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users", RegisterRequest{Email: email, Nickname: "race" + tag, Password: "pass1234"})
			statuses[index] = resp.StatusCode
			resp.Body.Close()
		}(i)
	}
	waitGroup.Wait()

	created, conflicted := 0, 0
	for _, status := range statuses {
		switch status {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			conflicted++
		default:
			t.Fatalf("unexpected status %d (all: %v)", status, statuses)
		}
	}
	if created != 1 || conflicted != 1 {
		t.Fatalf("created/conflicted = %d/%d, want 1/1", created, conflicted)
	}
}

func TestChangePasswordSucceedsAndSwapsCredentials(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, oldPassword := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: oldPassword})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	change := postJSON(t, client, ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: oldPassword, NewPassword: "new-pass-123"})
	if change.StatusCode != http.StatusOK {
		change.Body.Close()
		t.Fatalf("change status = %d, want 200", change.StatusCode)
	}
	var changeBody map[string]any
	if err := json.NewDecoder(change.Body).Decode(&changeBody); err != nil {
		t.Fatalf("decode change body: %v", err)
	}
	change.Body.Close()
	if len(changeBody) != 0 {
		t.Fatalf("change body = %v, want {}", changeBody)
	}

	row := userRow(t, db, uuid.MustParse(u.ID))
	if row.AuthVersion != 1 {
		t.Fatalf("auth_version = %d after change, want 1", row.AuthVersion)
	}
	if status := meStatus(t, client, ts.URL); status != http.StatusOK {
		t.Fatalf("current session me = %d, want 200", status)
	}

	oldLogin := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: oldPassword})
	oldLogin.Body.Close()
	if oldLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, want 401", oldLogin.StatusCode)
	}

	newLogin := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: "new-pass-123"})
	newLogin.Body.Close()
	if newLogin.StatusCode != http.StatusOK {
		t.Fatalf("new password login status = %d, want 200", newLogin.StatusCode)
	}
}

func TestChangePasswordRejectsBadInput(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)

	loginClient := func(t *testing.T) *http.Client {
		t.Helper()
		client := testClient(t)
		resp := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login status = %d, want 200", resp.StatusCode)
		}
		return client
	}

	t.Run("wrong current password", func(t *testing.T) {
		resp := postJSON(t, loginClient(t), ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: "wrong-pass-1", NewPassword: "new-pass-123"})
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		if got := decodeError(t, resp); got != "invalid_credentials" {
			t.Fatalf("error = %q, want invalid_credentials", got)
		}
	})

	t.Run("weak new password", func(t *testing.T) {
		resp := postJSON(t, loginClient(t), ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "abcdefgh"})
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		if got := decodeError(t, resp); got != "weak_password" {
			t.Fatalf("error = %q, want weak_password", got)
		}
	})

	t.Run("unchanged password", func(t *testing.T) {
		resp := postJSON(t, loginClient(t), ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: password})
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		if got := decodeError(t, resp); got != "password_unchanged" {
			t.Fatalf("error = %q, want password_unchanged", got)
		}
	})

	t.Run("anonymous", func(t *testing.T) {
		resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "new-pass-123"})
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		if got := decodeError(t, resp); got != "unauthorized" {
			t.Fatalf("error = %q, want unauthorized", got)
		}
	})
}

func TestChangePasswordOnDeletedUserReturnsUnauthorized(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()

	if err := db.Client.User.DeleteOneID(uuid.MustParse(u.ID)).Exec(context.Background()); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	resp := postJSON(t, client, ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "new-pass-123"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", got)
	}

	var cleared *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			cleared = c
		}
	}
	if cleared == nil || cleared.MaxAge >= 0 {
		t.Fatalf("expected the session cookie to be cleared, got %+v", cleared)
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	clientA := testClient(t)
	clientB := testClient(t)

	for _, client := range []*http.Client{clientA, clientB} {
		login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
		login.Body.Close()
		if login.StatusCode != http.StatusOK {
			t.Fatalf("login status = %d, want 200", login.StatusCode)
		}
	}

	change := postJSON(t, clientA, ts.URL+"/api/v1/users/me/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "new-pass-123"})
	change.Body.Close()
	if change.StatusCode != http.StatusOK {
		t.Fatalf("change status = %d, want 200", change.StatusCode)
	}

	meA, err := clientA.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me A: %v", err)
	}
	meA.Body.Close()
	if meA.StatusCode != http.StatusOK {
		t.Fatalf("client A status = %d, want 200 (current session must survive)", meA.StatusCode)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		meB, err := clientB.Get(ts.URL + "/api/v1/users/me")
		if err != nil {
			t.Fatalf("me B attempt %d: %v", attempt, err)
		}
		meB.Body.Close()
		if meB.StatusCode != http.StatusUnauthorized {
			t.Fatalf("client B attempt %d status = %d, want 401", attempt, meB.StatusCode)
		}
	}
}

func TestChangePasswordRejectsMalformedBody(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/sessions", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	resp := postRaw(t, client, ts.URL+"/api/v1/users/me/password", "{not json")
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("broken json status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("broken json error = %q, want invalid_request", got)
	}

	resp = postRaw(t, client, ts.URL+"/api/v1/users/me/password", `{"current_password":"`+password+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing fields status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("missing fields error = %q, want invalid_request", got)
	}
}
