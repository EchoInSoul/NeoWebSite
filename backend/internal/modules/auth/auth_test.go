package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"

	"website/backend/internal/common/guard"
	"website/backend/internal/database"
)

func newTestApp(t *testing.T, db *database.Database) *httptest.Server {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1")

	module := New(db, sm)
	module.Register(api)
	protected := api.Group("", guard.RequireAuth(sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
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

func decodeError(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()

	var body ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	return body.Error
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

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
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

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: "definitely-wrong"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password status = %d, want 401", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_credentials" {
		t.Fatalf("wrong password error = %q, want invalid_credentials", got)
	}

	resp = postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: "nobody+" + tag + "@example.com", Password: "whatever"})
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
			resp := postRaw(t, client, ts.URL+"/api/v1/auth/login", tc.body)
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

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: strings.Repeat("a", 73)})
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

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: strings.ToUpper(u.Email), Password: password})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestLoginRenewsSessionToken(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	first := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	first.Body.Close()
	before := sessionCookie(t, client.Jar, ts.URL)

	second := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	second.Body.Close()
	after := sessionCookie(t, client.Jar, ts.URL)

	if before == "" || after == "" {
		t.Fatalf("expected session cookies, got before=%q after=%q", before, after)
	}
	if before == after {
		t.Fatal("expected RenewToken to change the session cookie value")
	}
}

func TestMeRequiresSessionAndReturnsUser(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)

	anonymous := testClient(t)
	resp, err := anonymous.Get(ts.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("anonymous me: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", resp.StatusCode)
	}
	resp.Body.Close()

	client := testClient(t)
	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()

	resp, err = client.Get(ts.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body UserResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Email != u.Email {
		t.Fatalf("email = %q, want %q", body.Email, u.Email)
	}
}

func TestLogoutDestroysSession(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ts := newTestApp(t, db)
	client := testClient(t)

	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", nil)
	if err != nil {
		t.Fatalf("build logout: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("logout: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("logout status = %d, want 204", resp.StatusCode)
	}

	me, err := client.Get(ts.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("me after logout: %v", err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me after logout status = %d, want 401", me.StatusCode)
	}
}
