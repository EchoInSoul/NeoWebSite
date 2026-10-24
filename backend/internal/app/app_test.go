package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/rbac"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/modules/auth"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/settings"
)

func testConfig(t *testing.T, environment string) config.Config {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}

	return config.Config{
		Addr:              ":0",
		Environment:       environment,
		ServiceName:       "website-backend-test",
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
		SessionLifetime:   time.Hour,
		TOTPEncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	}
}

func newTestServer(t *testing.T, environment string) (*httptest.Server, *database.Database) {
	t.Helper()

	cfg := testConfig(t, environment)
	db, err := database.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	application, err := New(cfg, db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	ts := httptest.NewServer(application.Handler())
	t.Cleanup(ts.Close)
	return ts, db
}

func createTestUser(t *testing.T, db *database.Database) (auth.User, string) {
	t.Helper()

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	password := "pass-" + tag
	u, err := auth.CreateAdminUser(context.Background(), db, "admin+"+tag+"@test.local", "admin"+tag, password)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(uuid.MustParse(u.ID)).Exec(context.Background())
	})
	return u, password
}

func newClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func login(t *testing.T, client *http.Client, base, identifier, password string) *http.Response {
	t.Helper()

	payload, err := json.Marshal(map[string]string{"identifier": identifier, "password": password})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := client.Post(base+"/api/v1/sessions", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return resp
}

func loginWithLogout(t *testing.T, client *http.Client, ts *httptest.Server, identifier, password string) {
	t.Helper()

	resp := login(t, client, ts.URL, identifier, password)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}

	t.Cleanup(func() {
		request, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/sessions/current", nil)
		if err != nil {
			return
		}
		logoutResp, err := client.Do(request)
		if err != nil {
			return
		}
		logoutResp.Body.Close()
	})
}

func sessionCookie(resp *http.Response) *http.Cookie {
	for _, c := range resp.Cookies() {
		if c.Name == "session" {
			return c
		}
	}
	return nil
}

func TestAuthFlowEndToEnd(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)

	resp := login(t, client, ts.URL, u.Email, password)
	cookie := sessionCookie(resp)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	if cookie == nil {
		t.Fatal("expected a session cookie on login")
	}
	if !cookie.HttpOnly {
		t.Error("session cookie must be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Errorf("SameSite = %v, want Lax", cookie.SameSite)
	}
	if cookie.Path != "/" {
		t.Errorf("Path = %q, want /", cookie.Path)
	}
	if cookie.Secure {
		t.Error("development cookie must not be Secure")
	}

	me, err := client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want 200", me.StatusCode)
	}
	var meBody auth.UserResponse
	if err := json.NewDecoder(me.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	me.Body.Close()
	if meBody.Email != u.Email {
		t.Fatalf("me email = %q, want %q", meBody.Email, u.Email)
	}

	logoutReq, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/sessions/current", nil)
	if err != nil {
		t.Fatalf("build logout: %v", err)
	}
	logout, err := client.Do(logoutReq)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	logout.Body.Close()
	if logout.StatusCode != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", logout.StatusCode)
	}

	after, err := client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me after logout: %v", err)
	}
	after.Body.Close()
	if after.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want 401", after.StatusCode)
	}
}

func TestMeRevokesSessionWhenUserDeleted(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)

	resp := login(t, client, ts.URL, u.Email, password)
	resp.Body.Close()

	if err := db.Client.User.DeleteOneID(uuid.MustParse(u.ID)).Exec(context.Background()); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	me, err := client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", me.StatusCode)
	}

	cleared := sessionCookie(me)
	if cleared == nil {
		t.Fatal("expected the session cookie to be cleared")
	}
	if cleared.MaxAge >= 0 {
		t.Fatalf("session cookie MaxAge = %d, want negative", cleared.MaxAge)
	}
}

func TestProductionCookieIsSecure(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentProduction)
	u, password := createTestUser(t, db)
	client := newClient(t)

	resp := login(t, client, ts.URL, u.Email, password)
	cookie := sessionCookie(resp)
	resp.Body.Close()
	if cookie == nil {
		t.Fatal("expected a session cookie on login")
	}
	if !cookie.Secure {
		t.Error("production session cookie must be Secure")
	}
}

func TestGinModeFollowsEnvironment(t *testing.T) {
	for _, tc := range []struct {
		environment string
		want        string
	}{
		{config.EnvironmentProduction, gin.ReleaseMode},
		{config.EnvironmentDevelopment, gin.DebugMode},
	} {
		t.Run(tc.environment, func(t *testing.T) {
			mode, err := ginMode(tc.environment)
			if err != nil {
				t.Fatalf("ginMode(%q): %v", tc.environment, err)
			}
			if mode != tc.want {
				t.Errorf("gin mode = %q, want %q for environment %q", mode, tc.want, tc.environment)
			}
		})
	}

	for _, tc := range []struct {
		name        string
		environment string
	}{
		{"test", "test"},
		{"bogus", "bogus"},
		{"empty", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ginMode(tc.environment); err == nil {
				t.Fatalf("ginMode(%q) returned no error", tc.environment)
			}
		})
	}
}

func TestSessionCookieMaxAgeMatchesLifetime(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)

	resp := login(t, client, ts.URL, u.Email, password)
	cookie := sessionCookie(resp)
	resp.Body.Close()
	if cookie == nil {
		t.Fatal("expected a session cookie on login")
	}

	want := int(time.Hour.Seconds())
	if cookie.MaxAge != want && cookie.MaxAge != want+1 {
		t.Fatalf("cookie MaxAge = %d, want %d or %d (SessionLifetime set in testConfig; scs rounds Max-Age up, so a commit within the same clock tick yields %d)", cookie.MaxAge, want, want+1, want+1)
	}
}

func TestAnonymousLogoutRejected(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)
	client := newClient(t)

	req, err := http.NewRequest(http.MethodDelete, ts.URL+"/api/v1/sessions/current", nil)
	if err != nil {
		t.Fatalf("build logout: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestNoRouteReturnsErrorEnvelope(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)

	resp, err := http.Get(ts.URL + "/api/v1/nope")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "not_found" {
		t.Fatalf("error = %q, want not_found", body.Error.Code)
	}
}

type errorEnvelope struct {
	Error struct {
		Type  string  `json:"type"`
		Code  string  `json:"code"`
		Param *string `json:"param"`
	} `json:"error"`
}

func sendBare(t *testing.T, base, method, path string) (*http.Response, errorEnvelope) {
	t.Helper()

	req, err := http.NewRequest(method, base+path, nil)
	if err != nil {
		t.Fatalf("build %s %s: %v", method, path, err)
	}
	resp, err := newClient(t).Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	var body errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode %s %s: %v", method, path, err)
	}
	return resp, body
}

func sortedAllow(resp *http.Response) string {
	allow := strings.Split(resp.Header.Get("Allow"), ", ")
	sort.Strings(allow)
	return strings.Join(allow, ",")
}

func sortedMethods(methods ...string) string {
	sort.Strings(methods)
	return strings.Join(methods, ",")
}

func TestMethodMismatchReturns405Envelope(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)

	cases := []struct {
		name      string
		method    string
		path      string
		wantAllow string
	}{
		{"POST-only route hit with GET", http.MethodGet, "/api/v1/sessions", "POST"},
		{"GET-only route hit with DELETE", http.MethodDelete, "/api/v1/users/me/sessions", "GET"},
		{"GET+PATCH route hit with PUT", http.MethodPut, "/api/v1/users/me", sortedMethods("GET", "PATCH")},
		{"multi-method route hit with PUT", http.MethodPut, "/api/v1/admin/groups/" + uuid.NewString(), sortedMethods("PATCH", "DELETE")},
		{"OPTIONS is not registered anywhere", http.MethodOptions, "/api/v1/sessions", "POST"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := sendBare(t, ts.URL, tc.method, tc.path)
			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d, want 405 (body %+v)", resp.StatusCode, body)
			}
			if body.Error.Type != "invalid_request_error" {
				t.Fatalf("type = %q, want invalid_request_error", body.Error.Type)
			}
			if body.Error.Code != "method_not_allowed" {
				t.Fatalf("code = %q, want method_not_allowed", body.Error.Code)
			}
			if body.Error.Param != nil {
				t.Fatalf("param = %q, want null", *body.Error.Param)
			}
			if got := sortedAllow(resp); got != tc.wantAllow {
				t.Fatalf("Allow = %q, want %q", got, tc.wantAllow)
			}
		})
	}
}

func TestHeadMismatchSendsNoBody(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)

	req, err := http.NewRequest(http.MethodHead, ts.URL+"/api/v1/healthy", nil)
	if err != nil {
		t.Fatalf("build HEAD: %v", err)
	}
	resp, err := newClient(t).Do(req)
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", resp.StatusCode)
	}
	if resp.Header.Get("Allow") != "GET" {
		t.Fatalf("Allow = %q, want GET", resp.Header.Get("Allow"))
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if len(body) != 0 {
		t.Fatalf("HEAD must carry no body, got %q", body)
	}
}

func TestUnknownPathStaysNotFound(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)

	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodDelete} {
		resp, body := sendBare(t, ts.URL, method, "/api/v1/definitely/not/a/route")
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", method, resp.StatusCode)
		}
		if body.Error.Code != "not_found" {
			t.Fatalf("%s code = %q, want not_found", method, body.Error.Code)
		}
		if resp.Header.Get("Allow") != "" {
			t.Fatalf("%s unexpected Allow header %q", method, resp.Header.Get("Allow"))
		}
	}
}

func TestMatchingMethodIsNotHijackedByNoMethod(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)
	path := "/api/v1/admin/groups/" + uuid.NewString()

	wrong, wrongBody := sendBare(t, ts.URL, http.MethodPut, path)
	if wrong.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("PUT status = %d, want 405 (body %+v)", wrong.StatusCode, wrongBody)
	}

	right, rightBody := sendBare(t, ts.URL, http.MethodPatch, path)
	if right.StatusCode != http.StatusUnauthorized {
		t.Fatalf("PATCH status = %d, want 401 (body %+v)", right.StatusCode, rightBody)
	}
	if rightBody.Error.Code != "unauthorized" {
		t.Fatalf("PATCH code = %q, want unauthorized", rightBody.Error.Code)
	}
}

func TestPanicReturnsErrorEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(recoveryHandler())
	engine.GET("/boom", func(c *gin.Context) { panic("boom") })
	ts := httptest.NewServer(engine)
	t.Cleanup(ts.Close)

	resp, err := http.Get(ts.URL + "/boom")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "internal_error" {
		t.Fatalf("error = %q, want internal_error", body.Error.Code)
	}
}

func TestHealthyEndpointStillWorks(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)

	resp, err := http.Get(ts.URL + "/api/v1/healthy")
	if err != nil {
		t.Fatalf("healthy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["database"] != "ok" {
		t.Fatalf("database = %v, want ok", body["database"])
	}
}

func degradedConfig() config.Config {
	return config.Config{
		Addr:              ":0",
		Environment:       config.EnvironmentDevelopment,
		ServiceName:       "website-backend-test",
		DatabaseURL:       "postgresql://nobody:nobody@127.0.0.1:1/nowhere?sslmode=disable",
		DBMaxOpenConns:    2,
		DBMaxIdleConns:    1,
		DBConnMaxLifetime: time.Minute,
		SessionLifetime:   time.Hour,
		TOTPEncryptionKey: []byte("0123456789abcdef0123456789abcdef"),
	}
}

func newDegradedServer(t *testing.T) *httptest.Server {
	t.Helper()

	cfg := degradedConfig()
	db, err := database.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if db.StartupError() == nil {
		t.Fatal("expected the degraded fixture database url to be unreachable")
	}

	application, err := New(cfg, db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	ts := httptest.NewServer(application.Handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestDegradedServerStillServesRoutes(t *testing.T) {
	ts := newDegradedServer(t)

	cases := []struct {
		name     string
		method   string
		path     string
		body     string
		want     int
		wantCode string
	}{
		{"login needs the database", http.MethodPost, "/api/v1/sessions", `{"identifier":"a@b.c","password":"pass-12345"}`, http.StatusServiceUnavailable, "service_unavailable"},
		{"register needs the database", http.MethodPost, "/api/v1/users", `{"email":"a@b.c","nickname":"ab","password":"pass-12345"}`, http.StatusServiceUnavailable, "service_unavailable"},
		{"admin list needs the database", http.MethodPost, "/api/v1/admin/groups", `{"name":"G"}`, http.StatusUnauthorized, "unauthorized"},
		{"settings read needs the database", http.MethodGet, "/api/v1/admin/settings/register", "", http.StatusUnauthorized, "unauthorized"},
		{"public settings needs the database", http.MethodGet, "/api/v1/public/settings", "", http.StatusServiceUnavailable, "service_unavailable"},
		{"validation does not need the database", http.MethodPost, "/api/v1/sessions", `{}`, http.StatusBadRequest, "invalid_request"},
		{"unknown route is still 404", http.MethodGet, "/api/v1/definitely/not/a/route", "", http.StatusNotFound, "not_found"},
		{"method mismatch is still 405", http.MethodGet, "/api/v1/sessions", "", http.StatusMethodNotAllowed, "method_not_allowed"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, ts.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatalf("build request: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := newClient(t).Do(req)
			if err != nil {
				t.Fatalf("%s %s: %v", tc.method, tc.path, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.want {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.want)
			}
			var body errorEnvelope
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.Error.Code != tc.wantCode {
				t.Fatalf("code = %q, want %q", body.Error.Code, tc.wantCode)
			}
		})
	}
}

func TestDegradedServerReturns503ForStoredSessionCookie(t *testing.T) {
	ts := newDegradedServer(t)

	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/v1/users/me", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: "session", Value: "stale-token"})

	resp, err := newClient(t).Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var body errorEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != "service_unavailable" {
		t.Fatalf("code = %q, want service_unavailable", body.Error.Code)
	}

	anonymous, err := newClient(t).Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("anonymous request: %v", err)
	}
	defer anonymous.Body.Close()

	if anonymous.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymous.StatusCode)
	}
}

func TestDegradedHealthReportsDatabaseError(t *testing.T) {
	ts := newDegradedServer(t)

	resp, err := http.Get(ts.URL + "/api/v1/healthy")
	if err != nil {
		t.Fatalf("healthy: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "degraded" || body["database"] != "error" {
		t.Fatalf("body = %+v, want degraded / database error", body)
	}
}

func TestUpdateProfileEndToEnd(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)
	loginWithLogout(t, client, ts, u.Email, password)

	blockerTag := fmt.Sprintf("%d", time.Now().UnixNano())
	blocker, err := db.Client.User.Create().
		SetEmail("blocker+" + blockerTag + "@test.local").
		SetNickname("blocker" + blockerTag).
		SetPasswordHash("not-a-usable-hash").
		SetEmailVerificationStatus("not_required").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create blocker user: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(blocker.ID).Exec(context.Background())
	})

	endpoint := ts.URL + "/api/v1/users/me"
	profileTag := fmt.Sprintf("%d", time.Now().UnixNano())
	nickname := "prof" + profileTag
	email := "prof" + profileTag + "@test.local"

	status, body := exchangeJSON(t, newClient(t), http.MethodPatch, endpoint, map[string]any{"nickname": nickname})
	if status != http.StatusUnauthorized || envelopeCode(body) != "unauthorized" {
		t.Fatalf("anonymous patch = %d %v, want 401 unauthorized", status, body)
	}

	for _, absent := range []any{
		map[string]any{},
		json.RawMessage("null"),
		map[string]any{"nickname": nil},
		map[string]any{"bio": nil},
	} {
		status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, absent)
		if status != http.StatusBadRequest || envelopeCode(body) != "invalid_request" || envelopeParam(body) != "" {
			t.Fatalf("no field given %v = %d %v, want 400 invalid_request with no param", absent, status, body)
		}
	}

	for _, malformed := range []any{
		[]any{"nickname"},
		"nickname",
		42,
		map[string]any{"nickname": 42},
		map[string]any{"bio": 42},
	} {
		status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, malformed)
		if status != http.StatusBadRequest || envelopeCode(body) != "invalid_request" || envelopeParam(body) != "" {
			t.Fatalf("malformed %v = %d %v, want 400 invalid_request with no param", malformed, status, body)
		}
	}

	for _, invalid := range []map[string]any{
		{"nickname": "with space"},
		{"nickname": "with@at"},
		{"nickname": "  "},
		{"nickname": strings.Repeat("n", 33)},
		{"email": strings.Repeat("e", 250) + "@test.local"},
	} {
		status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, invalid)
		if status != http.StatusBadRequest || envelopeCode(body) != "invalid_request" {
			t.Fatalf("invalid %v = %d %v, want 400 invalid_request", invalid, status, body)
		}
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": "a"})
	if status != http.StatusBadRequest || envelopeCode(body) != "invalid_request" || envelopeParam(body) != "nickname" {
		t.Fatalf("short nickname = %d %v, want 400 invalid_request with param nickname", status, body)
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"email": "not-an-email"})
	if status != http.StatusBadRequest || envelopeCode(body) != "invalid_request" || envelopeParam(body) != "email" {
		t.Fatalf("bad email = %d %v, want 400 invalid_request with param email", status, body)
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{
		"nickname": "  " + nickname + "  ",
		"email":    strings.ToUpper(email),
	})
	if status != http.StatusOK || body["nickname"] != nickname || body["email"] != email {
		t.Fatalf("update = %d %v, want 200 trimmed nickname with lowercased email", status, body)
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": nickname, "email": email})
	if status != http.StatusOK {
		t.Fatalf("resubmit unchanged = %d %v, want 200", status, body)
	}

	assertProfile(t, client, ts, nickname, email)

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": blocker.Nickname})
	if status != http.StatusConflict || envelopeCode(body) != "nickname_taken" {
		t.Fatalf("taken nickname = %d %v, want 409 nickname_taken", status, body)
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"email": blocker.Email})
	if status != http.StatusConflict || envelopeCode(body) != "email_taken" {
		t.Fatalf("taken email = %d %v, want 409 email_taken", status, body)
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": nickname + "x"})
	if status != http.StatusOK || body["email"] != email {
		t.Fatalf("partial update = %d %v, want 200 keeping email", status, body)
	}

	emojiNickname := "🙂" + profileTag
	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": emojiNickname})
	if status != http.StatusOK || body["nickname"] != emojiNickname || body["email"] != email {
		t.Fatalf("emoji nickname = %d %v, want 200 keeping email", status, body)
	}
	assertProfile(t, client, ts, emojiNickname, email)

	wideEmojiNickname := strings.Repeat("🙂", 20)
	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": wideEmojiNickname})
	if status != http.StatusOK || body["nickname"] != wideEmojiNickname {
		t.Fatalf("20 rune emoji nickname = %d %v, want 200 counting runes not bytes", status, body)
	}
	assertProfile(t, client, ts, wideEmojiNickname, email)

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": "  " + wideEmojiNickname + "  "})
	if status != http.StatusOK || body["nickname"] != wideEmojiNickname {
		t.Fatalf("padded resubmit = %d %v, want 200 with trimmed self-match", status, body)
	}

	shortestNickname := "测试"
	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": shortestNickname})
	if status != http.StatusOK || body["nickname"] != shortestNickname {
		t.Fatalf("2 rune nickname = %d %v, want 200", status, body)
	}

	longestNickname := "nf" + profileTag + strings.Repeat("x", 11)
	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": longestNickname})
	if status != http.StatusOK || body["nickname"] != longestNickname {
		t.Fatalf("32 rune nickname = %d %v, want 200", status, body)
	}
	assertProfile(t, client, ts, longestNickname, email)

	longestEmail := strings.Repeat("e", 224) + profileTag + "@test.local"
	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"email": longestEmail})
	if status != http.StatusOK || body["email"] != longestEmail || body["nickname"] != longestNickname {
		t.Fatalf("254 byte email = %d %v, want 200 keeping nickname", status, body)
	}
	assertProfile(t, client, ts, longestNickname, longestEmail)
}

func TestUpdateProfileBio(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)
	loginWithLogout(t, client, ts, u.Email, password)

	endpoint := ts.URL + "/api/v1/users/me"

	status, me := exchangeJSON(t, client, http.MethodGet, endpoint, nil)
	if status != http.StatusOK {
		t.Fatalf("fresh me = %d %v, want 200", status, me)
	}
	if bio, present := me["bio"]; !present || bio != nil {
		t.Fatalf("fresh bio = %v (present=%v), want null", bio, present)
	}

	status, body := exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"bio": "  第一行\n第二行  "})
	if status != http.StatusOK || body["bio"] != "第一行\n第二行" {
		t.Fatalf("set bio = %d %v, want 200 with trimmed multiline bio", status, body)
	}
	if body["nickname"] != u.Nickname || body["email"] != u.Email {
		t.Fatalf("bio-only patch moved identity: %v", body)
	}
	assertBio(t, client, ts, "第一行\n第二行")
	assertStoredBio(t, db, u.ID, "第一行\n第二行")

	longestBio := strings.Repeat("长", 1000)
	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"bio": longestBio})
	if status != http.StatusOK || body["bio"] != longestBio {
		t.Fatalf("1000 rune bio = %d, want 200 counting runes not bytes", status)
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"bio": strings.Repeat("长", 1001)})
	if status != http.StatusBadRequest || envelopeCode(body) != "invalid_request" || envelopeParam(body) != "bio" {
		t.Fatalf("1001 rune bio = %d %v, want 400 invalid_request with param bio", status, body)
	}
	assertStoredBio(t, db, u.ID, longestBio)

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"bio": "   "})
	if status != http.StatusOK || body["bio"] != nil {
		t.Fatalf("blank bio = %d %v, want 200 with null bio", status, body)
	}
	assertBio(t, client, ts, "")
	assertStoredBio(t, db, u.ID, "")

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"bio": "keep-me"})
	if status != http.StatusOK || body["bio"] != "keep-me" {
		t.Fatalf("reset bio = %d %v, want 200", status, body)
	}

	renamed := u.Nickname + "b"
	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": renamed})
	if status != http.StatusOK || body["bio"] != "keep-me" || body["nickname"] != renamed {
		t.Fatalf("nickname-only patch = %d %v, want bio kept", status, body)
	}

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"bio": ""})
	if status != http.StatusOK || body["bio"] != nil {
		t.Fatalf("explicit clear = %d %v, want 200 with null bio", status, body)
	}
	assertBio(t, client, ts, "")
	assertStoredBio(t, db, u.ID, "")
}

func TestUpdateProfileRejectedWriteLeavesBothFieldsUnchanged(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)
	loginWithLogout(t, client, ts, u.Email, password)

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	blocker, err := db.Client.User.Create().
		SetEmail("blocker+" + tag + "@test.local").
		SetNickname("blocker" + tag).
		SetPasswordHash("not-a-usable-hash").
		SetEmailVerificationStatus("not_required").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create blocker user: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(blocker.ID).Exec(context.Background())
	})

	endpoint := ts.URL + "/api/v1/users/me"
	freeNickname := "free" + tag
	freeEmail := "free" + tag + "@test.local"

	status, body := exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{
		"nickname": freeNickname,
		"email":    blocker.Email,
	})
	if status != http.StatusConflict || envelopeCode(body) != "email_taken" {
		t.Fatalf("free nickname with taken email = %d %v, want 409 email_taken", status, body)
	}
	assertProfile(t, client, ts, u.Nickname, u.Email)

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{
		"nickname": blocker.Nickname,
		"email":    freeEmail,
	})
	if status != http.StatusConflict || envelopeCode(body) != "nickname_taken" {
		t.Fatalf("taken nickname with free email = %d %v, want 409 nickname_taken", status, body)
	}
	assertProfile(t, client, ts, u.Nickname, u.Email)

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"email": freeEmail})
	if status != http.StatusOK || body["email"] != freeEmail || body["nickname"] != u.Nickname {
		t.Fatalf("email only update = %d %v, want 200 with nickname kept", status, body)
	}
	assertProfile(t, client, ts, u.Nickname, freeEmail)

	status, body = exchangeJSON(t, client, http.MethodPatch, endpoint, map[string]any{"nickname": freeNickname})
	if status != http.StatusOK || body["nickname"] != freeNickname || body["email"] != freeEmail {
		t.Fatalf("nickname only update = %d %v, want 200 with email kept", status, body)
	}
	assertProfile(t, client, ts, freeNickname, freeEmail)
}

func TestUpdateProfileConcurrentSameNickname(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	first, firstPassword := createTestUser(t, db)
	second, secondPassword := createTestUser(t, db)

	firstClient := newClient(t)
	loginWithLogout(t, firstClient, ts, first.Email, firstPassword)
	secondClient := newClient(t)
	loginWithLogout(t, secondClient, ts, second.Email, secondPassword)

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	endpoint := ts.URL + "/api/v1/users/me"

	type outcome struct {
		status int
		code   string
		err    error
	}

	results := make([]outcome, 2)
	var waitGroup sync.WaitGroup
	waitGroup.Add(2)
	for index, client := range []*http.Client{firstClient, secondClient} {
		go func(index int, client *http.Client) {
			defer waitGroup.Done()
			status, body, err := patchJSON(client, endpoint, map[string]any{"nickname": "race" + tag})
			results[index] = outcome{status: status, code: envelopeCode(body), err: err}
		}(index, client)
	}
	waitGroup.Wait()

	succeeded, conflicted := 0, 0
	for _, result := range results {
		if result.err != nil {
			t.Fatalf("concurrent patch: %v", result.err)
		}
		switch result.status {
		case http.StatusOK:
			succeeded++
		case http.StatusConflict:
			conflicted++
			if result.code != "nickname_taken" && result.code != "already_taken" {
				t.Fatalf("conflict code = %q, want nickname_taken or already_taken", result.code)
			}
		default:
			t.Fatalf("unexpected status %d (all: %v)", result.status, results)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded/conflicted = %d/%d, want 1/1", succeeded, conflicted)
	}
}

func TestUpdateProfileConstraintBackstopReturnsAlreadyTaken(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)
	loginWithLogout(t, client, ts, u.Email, password)

	installNicknameCollisionTrigger(t, db)

	sentinel := "collide" + fmt.Sprintf("%d", time.Now().UnixNano())
	status, body := exchangeJSON(t, client, http.MethodPatch, ts.URL+"/api/v1/users/me", map[string]any{"nickname": sentinel})
	if status != http.StatusConflict || envelopeCode(body) != "already_taken" {
		t.Fatalf("constraint backstop = %d %v, want 409 already_taken", status, body)
	}
	assertProfile(t, client, ts, u.Nickname, u.Email)
}

func installNicknameCollisionTrigger(t *testing.T, db *database.Database) {
	t.Helper()

	if _, err := db.SQL().Exec("DROP TRIGGER IF EXISTS neoblog_test_nickname_collision ON users"); err != nil {
		t.Fatalf("drop stale collision trigger: %v", err)
	}
	if _, err := db.SQL().Exec(`
CREATE OR REPLACE FUNCTION neoblog_test_nickname_collision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF NEW.nickname LIKE 'collide%' THEN
		INSERT INTO users (id, email, nickname, password_hash, email_verification_status, group_id, created_at, updated_at)
		VALUES (gen_random_uuid(), NEW.nickname || '@collision.test', NEW.nickname, 'not-a-usable-hash', 'not_required', NEW.group_id, now(), now());
	END IF;
	RETURN NEW;
END $$`); err != nil {
		t.Fatalf("create collision function: %v", err)
	}
	if _, err := db.SQL().Exec(`
CREATE TRIGGER neoblog_test_nickname_collision BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION neoblog_test_nickname_collision()`); err != nil {
		t.Fatalf("create collision trigger: %v", err)
	}

	t.Cleanup(func() {
		_, _ = db.SQL().Exec("DROP TRIGGER IF EXISTS neoblog_test_nickname_collision ON users")
		_, _ = db.SQL().Exec("DROP FUNCTION IF EXISTS neoblog_test_nickname_collision()")
		_, _ = db.SQL().Exec("DELETE FROM users WHERE email LIKE '%@collision.test'")
	})
}

func TestChangePasswordToleratesBadSessionRows(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	u, password := createTestUser(t, db)
	client := newClient(t)

	loginResp := login(t, client, ts.URL, u.Email, password)
	loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResp.StatusCode)
	}

	junkToken := fmt.Sprintf("junk-%d", time.Now().UnixNano())
	if _, err := db.SQL().Exec(
		"INSERT INTO sessions (token, data, expiry) VALUES ($1, $2, now() + interval '1 hour')",
		junkToken, []byte{0},
	); err != nil {
		t.Fatalf("insert junk session: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.SQL().Exec("DELETE FROM sessions WHERE token = $1", junkToken)
	})

	payload, err := json.Marshal(map[string]string{"current_password": password, "new_password": "new-pass-123"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := client.Post(ts.URL+"/api/v1/users/me/password", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("change password: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestShutdownClosesMailer(t *testing.T) {
	cfg := testConfig(t, config.EnvironmentDevelopment)
	db, err := database.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	a, err := New(cfg, db)
	if err != nil {
		t.Fatalf("new app: %v", err)
	}
	if err := a.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	err = a.mailer.Send(context.Background(), mail.Message{To: "user@example.com", Subject: "s", Body: mail.Body{Text: "t"}})
	if !errors.Is(err, mail.ErrClosed) {
		t.Fatalf("send after shutdown = %v, want mail.ErrClosed", err)
	}
}

func exchangeJSON(t *testing.T, client *http.Client, method, endpoint string, body any) (int, map[string]any) {
	t.Helper()

	var payload []byte
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		payload = encoded
	}

	request, err := http.NewRequest(method, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, endpoint, err)
	}
	defer resp.Body.Close()

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s %s: %v", method, endpoint, err)
	}
	return resp.StatusCode, decoded
}

func envelopeValues(decoded map[string]any) map[string]any {
	values, _ := decoded["values"].(map[string]any)
	return values
}

func envelopeCode(decoded map[string]any) string {
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := errObj["code"].(string)
	return code
}

func envelopeParam(decoded map[string]any) string {
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		return ""
	}
	param, _ := errObj["param"].(string)
	return param
}

func assertProfile(t *testing.T, client *http.Client, ts *httptest.Server, nickname, email string) {
	t.Helper()

	me, err := client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want 200", me.StatusCode)
	}

	var meBody map[string]any
	if err := json.NewDecoder(me.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if meBody["nickname"] != nickname || meBody["email"] != email {
		t.Fatalf("profile = %v, want %s / %s", meBody, nickname, email)
	}
}

func assertBio(t *testing.T, client *http.Client, ts *httptest.Server, want string) {
	t.Helper()

	me, err := client.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer me.Body.Close()

	var meBody map[string]any
	if err := json.NewDecoder(me.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	got, present := meBody["bio"]
	if want == "" {
		if !present || got != nil {
			t.Fatalf("bio = %v (present=%v), want null", got, present)
		}
		return
	}
	if got != want {
		t.Fatalf("bio = %v, want %q", got, want)
	}
}

func assertStoredBio(t *testing.T, db *database.Database, userID, want string) {
	t.Helper()

	stored, err := db.Client.User.Get(context.Background(), uuid.MustParse(userID))
	if err != nil {
		t.Fatalf("load stored user: %v", err)
	}
	if want == "" {
		if stored.Bio != nil {
			t.Fatalf("stored bio = %q, want NULL", *stored.Bio)
		}
		return
	}
	if stored.Bio == nil || *stored.Bio != want {
		t.Fatalf("stored bio = %v, want %q", stored.Bio, want)
	}
}

func patchJSON(client *http.Client, endpoint string, body any) (int, map[string]any, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}

	request, err := http.NewRequest(http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()

	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, decoded, nil
}

func TestRegistrationControlEndToEnd(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	owner, ownerPassword := createGroupUser(t, db, rbac.GroupIDOwner)

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	email := "regctl+" + tag + "@example.com"
	regBody := map[string]any{"email": email, "nickname": "regctl" + tag, "password": "pass-12345"}
	endpoint := ts.URL + "/api/v1/admin/settings/register"

	resetRegister := func() error {
		module, err := settings.New(db)
		if err != nil {
			return err
		}
		for _, name := range []string{
			"enabled",
			"default_group",
			"email_domain_control_enabled",
			"allowed_email_domains",
			"email_verification_enabled",
		} {
			if err := module.Service.ResetItem(context.Background(), "register", name); err != nil {
				return err
			}
		}
		return nil
	}
	if err := resetRegister(); err != nil {
		t.Fatalf("reset register settings: %v", err)
	}
	t.Cleanup(func() {
		_ = resetRegister()
		_, _ = db.SQL().Exec("DELETE FROM users WHERE email = $1", email)
	})

	ownerClient := newClient(t)
	loginResp := login(t, ownerClient, ts.URL, owner.Email, ownerPassword)
	if loginResp.StatusCode != http.StatusOK {
		loginResp.Body.Close()
		t.Fatalf("owner login status = %d, want 200", loginResp.StatusCode)
	}
	loginResp.Body.Close()

	status, values := exchangeJSON(t, ownerClient, http.MethodGet, endpoint, nil)
	if status != http.StatusOK || values["group"] != "register" || envelopeValues(values)["enabled"] != true {
		t.Fatalf("default get = %d %v, want 200 register with enabled true", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{"enabled": false})
	if status != http.StatusOK || envelopeValues(values)["enabled"] != false {
		t.Fatalf("close patch = %d %v, want 200 with enabled false", status, values)
	}

	anonymous := newClient(t)
	status, values = exchangeJSON(t, anonymous, http.MethodPost, ts.URL+"/api/v1/users", regBody)
	if status != http.StatusForbidden || envelopeCode(values) != "registration_closed" {
		t.Fatalf("closed register = %d %v, want 403 registration_closed", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{"enabled": true})
	if status != http.StatusOK || envelopeValues(values)["enabled"] != true {
		t.Fatalf("reopen patch = %d %v, want 200 with enabled true", status, values)
	}

	status, values = exchangeJSON(t, anonymous, http.MethodPost, ts.URL+"/api/v1/users", regBody)
	if status != http.StatusCreated {
		t.Fatalf("reopened register = %d %v, want 201", status, values)
	}
}

func TestMailSettingsEndToEnd(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	owner, ownerPassword := createGroupUser(t, db, rbac.GroupIDOwner)

	endpoint := ts.URL + "/api/v1/admin/settings/mail"
	resetMail := func() error {
		module, err := settings.New(db)
		if err != nil {
			return err
		}
		for _, name := range []string{
			"enabled",
			"smtp_host",
			"smtp_port",
			"smtp_encryption",
			"smtp_username",
			"smtp_password",
			"sender_address",
			"sender_name",
			"signup_code_subject",
			"signup_code_body",
			"password_reset_subject",
			"password_reset_body",
		} {
			if err := module.Service.ResetItem(context.Background(), "mail", name); err != nil {
				return err
			}
		}
		return nil
	}
	if err := resetMail(); err != nil {
		t.Fatalf("reset mail settings: %v", err)
	}
	t.Cleanup(func() { _ = resetMail() })

	ownerClient := newClient(t)
	loginResp := login(t, ownerClient, ts.URL, owner.Email, ownerPassword)
	if loginResp.StatusCode != http.StatusOK {
		loginResp.Body.Close()
		t.Fatalf("owner login status = %d, want 200", loginResp.StatusCode)
	}
	loginResp.Body.Close()

	status, values := exchangeJSON(t, ownerClient, http.MethodGet, endpoint, nil)
	if status != http.StatusOK || values["group"] != "mail" {
		t.Fatalf("default get = %d %v, want 200 mail group", status, values)
	}
	defaults := envelopeValues(values)
	if defaults["enabled"] != true || defaults["smtp_port"] != float64(587) || defaults["smtp_encryption"] != "starttls" || defaults["sender_name"] != "" {
		t.Fatalf("default values = %v, want enabled true, port 587, starttls, empty sender_name", defaults)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPost, ts.URL+"/api/v1/admin/mail/messages", map[string]any{
		"to":       owner.Email,
		"template": "signup_code",
	})
	if status != http.StatusCreated || values["subject"] != "注册验证邮件" {
		t.Fatalf("template send = %d %v, want 201 with default signup subject", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{
		"enabled":         true,
		"smtp_host":       "smtp.example.com",
		"smtp_port":       465,
		"smtp_encryption": "ssl",
		"smtp_username":   "sender@example.com",
		"smtp_password":   "secret",
		"sender_address":  "sender@example.com",
		"sender_name":     "Example",
	})
	if status != http.StatusOK {
		t.Fatalf("valid patch = %d %v, want 200", status, values)
	}
	patched := envelopeValues(values)
	if patched["smtp_host"] != "smtp.example.com" || patched["smtp_port"] != float64(465) || patched["smtp_encryption"] != "ssl" || patched["smtp_password"] != "secret" {
		t.Fatalf("patched values = %v, want stored SMTP settings", patched)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{"sender_address": ""})
	if status != http.StatusConflict || envelopeCode(values) != "setting_conflict" || envelopeParam(values) != "sender_address" {
		t.Fatalf("missing sender = %d %v, want 409 setting_conflict sender_address", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{"smtp_password": ""})
	if status != http.StatusConflict || envelopeCode(values) != "setting_conflict" || envelopeParam(values) != "smtp_password" {
		t.Fatalf("missing password = %d %v, want 409 setting_conflict smtp_password", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{"smtp_port": 70000})
	if status != http.StatusConflict || envelopeCode(values) != "setting_conflict" || envelopeParam(values) != "smtp_port" {
		t.Fatalf("bad port = %d %v, want 409 setting_conflict smtp_port", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodGet, endpoint, nil)
	if status != http.StatusOK || envelopeValues(values)["smtp_host"] != "smtp.example.com" {
		t.Fatalf("readback = %d %v, want stored smtp_host", status, values)
	}
}

func createGroupUser(t *testing.T, db *database.Database, groupID uuid.UUID) (auth.User, string) {
	t.Helper()

	u, password := createTestUser(t, db)
	if err := db.Client.User.UpdateOneID(uuid.MustParse(u.ID)).SetGroupID(groupID).Exec(context.Background()); err != nil {
		t.Fatalf("set group: %v", err)
	}
	return u, password
}

type meAccessBody struct {
	Group struct {
		SystemKey *string `json:"system_key"`
		Name      string  `json:"name"`
	} `json:"group"`
	Permissions []string `json:"permissions"`
}

func TestAccessEndToEnd(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	tag := fmt.Sprintf("%d", time.Now().UnixNano())

	plainUser, plainPassword := createGroupUser(t, db, rbac.GroupIDUser)
	ownerUser, ownerPassword := createGroupUser(t, db, rbac.GroupIDOwner)

	anonymousResp, err := newClient(t).Get(ts.URL + "/api/v1/admin/users")
	if err != nil {
		t.Fatalf("anonymous admin get: %v", err)
	}
	anonymousResp.Body.Close()
	if anonymousResp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymousResp.StatusCode)
	}

	plainClient := newClient(t)
	plainLogin := login(t, plainClient, ts.URL, plainUser.Email, plainPassword)
	plainLogin.Body.Close()

	plainAdmin, err := plainClient.Get(ts.URL + "/api/v1/admin/users")
	if err != nil {
		t.Fatalf("plain admin get: %v", err)
	}
	plainAdmin.Body.Close()
	if plainAdmin.StatusCode != http.StatusForbidden {
		t.Fatalf("plain admin status = %d, want 403", plainAdmin.StatusCode)
	}

	me, err := plainClient.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("plain me: %v", err)
	}
	var meBody meAccessBody
	if err := json.NewDecoder(me.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	me.Body.Close()
	if meBody.Group.SystemKey == nil || *meBody.Group.SystemKey != rbac.GroupKeyUser {
		t.Fatalf("plain group = %+v, want user preset", meBody.Group)
	}
	if len(meBody.Permissions) != 0 {
		t.Fatalf("plain permissions = %v, want empty", meBody.Permissions)
	}

	ownerClient := newClient(t)
	ownerLogin := login(t, ownerClient, ts.URL, ownerUser.Email, ownerPassword)
	ownerLogin.Body.Close()

	ownerAdmin, err := ownerClient.Get(ts.URL + "/api/v1/admin/users")
	if err != nil {
		t.Fatalf("owner admin get: %v", err)
	}
	ownerAdmin.Body.Close()
	if ownerAdmin.StatusCode != http.StatusOK {
		t.Fatalf("owner admin status = %d, want 200", ownerAdmin.StatusCode)
	}

	groupPayload, err := json.Marshal(map[string]any{
		"name":        "编辑-" + tag,
		"permissions": []string{rbac.PermissionUsersView},
	})
	if err != nil {
		t.Fatalf("marshal group: %v", err)
	}
	createResp, err := ownerClient.Post(ts.URL+"/api/v1/admin/groups", "application/json", bytes.NewReader(groupPayload))
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create group status = %d, want 201", createResp.StatusCode)
	}
	var groupBody struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&groupBody); err != nil {
		t.Fatalf("decode group: %v", err)
	}
	createResp.Body.Close()
	t.Cleanup(func() {
		_ = db.Client.User.UpdateOneID(uuid.MustParse(plainUser.ID)).SetGroupID(rbac.GroupIDUser).Exec(context.Background())
		_ = db.Client.Group.DeleteOneID(uuid.MustParse(groupBody.ID)).Exec(context.Background())
	})

	patchPayload, err := json.Marshal(map[string]any{"group_id": groupBody.ID})
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	patchReq, err := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/admin/users/"+plainUser.ID, bytes.NewReader(patchPayload))
	if err != nil {
		t.Fatalf("build patch: %v", err)
	}
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := ownerClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch user: %v", err)
	}
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", patchResp.StatusCode)
	}

	afterMe, err := plainClient.Get(ts.URL + "/api/v1/users/me")
	if err != nil {
		t.Fatalf("me after group change: %v", err)
	}
	var afterBody meAccessBody
	if err := json.NewDecoder(afterMe.Body).Decode(&afterBody); err != nil {
		t.Fatalf("decode me after: %v", err)
	}
	afterMe.Body.Close()
	if afterBody.Group.SystemKey != nil {
		t.Fatalf("custom group system key = %v, want null", *afterBody.Group.SystemKey)
	}
	if len(afterBody.Permissions) != 1 || afterBody.Permissions[0] != rbac.PermissionUsersView {
		t.Fatalf("permissions after group change = %v, want [users.view]", afterBody.Permissions)
	}
}

func TestMailMessagesEndpointEndToEnd(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)

	postMessage := func(client *http.Client) *http.Response {
		t.Helper()
		resp, err := client.Post(ts.URL+"/api/v1/admin/mail/messages", "application/json", bytes.NewReader([]byte(`{"to":"someone@example.com"}`)))
		if err != nil {
			t.Fatalf("post messages: %v", err)
		}
		return resp
	}

	anonymous := postMessage(newClient(t))
	anonymous.Body.Close()
	if anonymous.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anonymous.StatusCode)
	}

	plainUser, plainPassword := createGroupUser(t, db, rbac.GroupIDUser)
	ownerUser, ownerPassword := createGroupUser(t, db, rbac.GroupIDOwner)

	plainClient := newClient(t)
	plainLogin := login(t, plainClient, ts.URL, plainUser.Email, plainPassword)
	plainLogin.Body.Close()

	plain := postMessage(plainClient)
	plain.Body.Close()
	if plain.StatusCode != http.StatusForbidden {
		t.Fatalf("plain user status = %d, want 403", plain.StatusCode)
	}

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	groupPayload, err := json.Marshal(map[string]any{"name": "MailCustom-" + tag, "permissions": []string{}})
	if err != nil {
		t.Fatalf("marshal group: %v", err)
	}

	ownerClient := newClient(t)
	ownerLogin := login(t, ownerClient, ts.URL, ownerUser.Email, ownerPassword)
	ownerLogin.Body.Close()

	createResp, err := ownerClient.Post(ts.URL+"/api/v1/admin/groups", "application/json", bytes.NewReader(groupPayload))
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if createResp.StatusCode != http.StatusCreated {
		createResp.Body.Close()
		t.Fatalf("create group status = %d, want 201", createResp.StatusCode)
	}
	var groupBody struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(createResp.Body).Decode(&groupBody); err != nil {
		t.Fatalf("decode group: %v", err)
	}
	createResp.Body.Close()
	t.Cleanup(func() {
		_ = db.Client.User.UpdateOneID(uuid.MustParse(plainUser.ID)).SetGroupID(rbac.GroupIDUser).Exec(context.Background())
		_ = db.Client.Group.DeleteOneID(uuid.MustParse(groupBody.ID)).Exec(context.Background())
	})

	patchPayload, err := json.Marshal(map[string]any{"group_id": groupBody.ID})
	if err != nil {
		t.Fatalf("marshal patch: %v", err)
	}
	patchReq, err := http.NewRequest(http.MethodPatch, ts.URL+"/api/v1/admin/users/"+plainUser.ID, bytes.NewReader(patchPayload))
	if err != nil {
		t.Fatalf("build patch: %v", err)
	}
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := ownerClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch user: %v", err)
	}
	patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", patchResp.StatusCode)
	}

	custom := postMessage(plainClient)
	custom.Body.Close()
	if custom.StatusCode != http.StatusForbidden {
		t.Fatalf("custom group status = %d, want 403", custom.StatusCode)
	}

	owner := postMessage(ownerClient)
	if owner.StatusCode != http.StatusCreated {
		owner.Body.Close()
		t.Fatalf("owner status = %d, want 201", owner.StatusCode)
	}
	var sentBody struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Sent    bool   `json:"sent"`
	}
	if err := json.NewDecoder(owner.Body).Decode(&sentBody); err != nil {
		t.Fatalf("decode sent: %v", err)
	}
	owner.Body.Close()
	if sentBody.To != "someone@example.com" || !sentBody.Sent || sentBody.Subject == "" {
		t.Fatalf("sent body = %+v, want to/sent/subject", sentBody)
	}
}
