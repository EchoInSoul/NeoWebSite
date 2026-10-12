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
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/rbac"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/modules/auth"
	"website/backend/internal/modules/mail"
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
	resp, err := client.Post(base+"/api/v1/auth/login", "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return resp
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

	me, err := client.Get(ts.URL + "/api/v1/auth/me")
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

	logoutReq, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", nil)
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

	after, err := client.Get(ts.URL + "/api/v1/auth/me")
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

	me, err := client.Get(ts.URL + "/api/v1/auth/me")
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
	previous := gin.Mode()
	t.Cleanup(func() { gin.SetMode(previous) })

	for _, tc := range []struct {
		environment string
		want        string
	}{
		{config.EnvironmentProduction, gin.ReleaseMode},
		{config.EnvironmentDevelopment, gin.DebugMode},
		{config.EnvironmentTest, gin.TestMode},
		{"bogus", gin.ReleaseMode},
	} {
		t.Run(tc.environment, func(t *testing.T) {
			gin.SetMode(ginMode(tc.environment))
			if got := gin.Mode(); got != tc.want {
				t.Errorf("gin mode = %q, want %q for environment %q", got, tc.want, tc.environment)
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
	if cookie.MaxAge != want {
		t.Fatalf("cookie MaxAge = %d, want %d (SessionLifetime set in testConfig)", cookie.MaxAge, want)
	}
}

func TestAnonymousLogoutRejected(t *testing.T) {
	ts, _ := newTestServer(t, config.EnvironmentDevelopment)
	client := newClient(t)

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", nil)
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
		{"POST-only route hit with GET", http.MethodGet, "/api/v1/auth/login", "POST"},
		{"GET-only route hit with DELETE", http.MethodDelete, "/api/v1/auth/me", "GET"},
		{"multi-method route hit with PUT", http.MethodPut, "/api/v1/admin/groups/" + uuid.NewString(), sortedMethods("PATCH", "DELETE")},
		{"OPTIONS is not registered anywhere", http.MethodOptions, "/api/v1/auth/login", "POST"},
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
		{"login needs the database", http.MethodPost, "/api/v1/auth/login", `{"identifier":"a@b.c","password":"pass-12345"}`, http.StatusServiceUnavailable, "service_unavailable"},
		{"register needs the database", http.MethodPost, "/api/v1/auth/register", `{"email":"a@b.c","nickname":"ab","password":"pass-12345"}`, http.StatusServiceUnavailable, "service_unavailable"},
		{"admin list needs the database", http.MethodPost, "/api/v1/admin/groups", `{"name":"G"}`, http.StatusUnauthorized, "unauthorized"},
		{"settings read needs the database", http.MethodGet, "/api/v1/admin/settings/register", "", http.StatusUnauthorized, "unauthorized"},
		{"validation does not need the database", http.MethodPost, "/api/v1/auth/login", `{}`, http.StatusBadRequest, "invalid_request"},
		{"unknown route is still 404", http.MethodGet, "/api/v1/definitely/not/a/route", "", http.StatusNotFound, "not_found"},
		{"method mismatch is still 405", http.MethodGet, "/api/v1/auth/login", "", http.StatusMethodNotAllowed, "method_not_allowed"},
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
	resp, err := client.Post(ts.URL+"/api/v1/auth/password", "application/json", bytes.NewReader(payload))
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

func envelopeCode(decoded map[string]any) string {
	errObj, ok := decoded["error"].(map[string]any)
	if !ok {
		return ""
	}
	code, _ := errObj["code"].(string)
	return code
}

func TestRegistrationControlEndToEnd(t *testing.T) {
	ts, db := newTestServer(t, config.EnvironmentDevelopment)
	owner, ownerPassword := createGroupUser(t, db, rbac.GroupIDOwner)

	tag := fmt.Sprintf("%d", time.Now().UnixNano())
	email := "regctl+" + tag + "@example.com"
	regBody := map[string]any{"email": email, "nickname": "regctl" + tag, "password": "pass-12345"}
	endpoint := ts.URL + "/api/v1/admin/settings/register"

	clearSettings := func() {
		if _, err := db.SQL().Exec("DELETE FROM setting_values WHERE setting_group = $1", "register"); err != nil {
			t.Fatalf("clear register settings: %v", err)
		}
	}
	clearSettings()
	t.Cleanup(func() {
		_, _ = db.SQL().Exec("DELETE FROM setting_values WHERE setting_group = $1", "register")
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
	if status != http.StatusOK || values["enabled"] != true {
		t.Fatalf("default get = %d %v, want 200 with enabled true", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{"enabled": false})
	if status != http.StatusOK || values["enabled"] != false {
		t.Fatalf("close patch = %d %v, want 200 with enabled false", status, values)
	}

	anonymous := newClient(t)
	status, values = exchangeJSON(t, anonymous, http.MethodPost, ts.URL+"/api/v1/auth/register", regBody)
	if status != http.StatusForbidden || envelopeCode(values) != "registration_closed" {
		t.Fatalf("closed register = %d %v, want 403 registration_closed", status, values)
	}

	status, values = exchangeJSON(t, ownerClient, http.MethodPatch, endpoint, map[string]any{"enabled": true})
	if status != http.StatusOK || values["enabled"] != true {
		t.Fatalf("reopen patch = %d %v, want 200 with enabled true", status, values)
	}

	status, values = exchangeJSON(t, anonymous, http.MethodPost, ts.URL+"/api/v1/auth/register", regBody)
	if status != http.StatusCreated {
		t.Fatalf("reopened register = %d %v, want 201", status, values)
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

	me, err := plainClient.Get(ts.URL + "/api/v1/auth/me")
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

	afterMe, err := plainClient.Get(ts.URL + "/api/v1/auth/me")
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
