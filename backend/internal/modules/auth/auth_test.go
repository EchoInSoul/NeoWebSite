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
	"website/backend/internal/database"
	"website/backend/internal/database/ent/user"
	"website/backend/internal/modules/access"
)

func newTestAppWithSessions(t *testing.T, db *database.Database) (*httptest.Server, *scs.SessionManager) {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	api := engine.Group("/api/v1")

	module := New(db, sm, testTOTPKey)
	module.Register(api)
	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.LoadIdentity(accessModule.Service, sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, sm
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

	me, err := client.Get(ts.URL + "/api/v1/auth/me")
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

	first := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	first.Body.Close()
	if first.StatusCode != http.StatusOK {
		t.Fatalf("first login status = %d, want 200", first.StatusCode)
	}

	sm.Store = deleteFailStore{sm.Store}

	second := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
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

	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	sm.Store = deleteFailStore{sm.Store}

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/v1/auth/logout", nil)
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

func TestRegisterSucceedsAndLogsIn(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts := newTestApp(t, db)
	client := testClient(t)

	email := "New+" + tag + "@Example.COM"
	cleanupUserByEmail(t, db, strings.ToLower(email))

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: email, Nickname: "new" + tag, Password: "pass1234"})
	cookie := sessionCookie(t, client.Jar, ts.URL)
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
	if cookie == "" {
		t.Fatal("expected a session cookie")
	}

	me, err := client.Get(ts.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer me.Body.Close()
	if me.StatusCode != http.StatusOK {
		t.Fatalf("me status = %d, want 200", me.StatusCode)
	}
	var meBody UserResponse
	if err := json.NewDecoder(me.Body).Decode(&meBody); err != nil {
		t.Fatalf("decode me: %v", err)
	}
	if meBody.Email != strings.ToLower(email) || meBody.Nickname != "new"+tag {
		t.Fatalf("me = %+v, want %s / new%s", meBody, strings.ToLower(email), tag)
	}

	paddedEmail := "padded+" + tag + "@example.com"
	cleanupUserByEmail(t, db, paddedEmail)
	padded := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: " PADDED+" + tag + "@EXAMPLE.COM ", Nickname: "  pad" + tag + "  ", Password: "pass1234"})
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

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: strings.ToUpper(u.Email), Nickname: "other" + tag, Password: "pass1234"})
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
	resp := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: email, Nickname: u.Nickname, Password: "pass1234"})
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
			resp := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: "weak+" + tag + "@example.com", Nickname: "weak" + tag, Password: tc.password})
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
			resp := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: tc.email, Nickname: tc.nickname, Password: "pass1234"})
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

	resp := postRaw(t, client, ts.URL+"/api/v1/auth/register", "{not json")
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("broken json status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("broken json error = %q, want invalid_request", got)
	}

	resp = postRaw(t, client, ts.URL+"/api/v1/auth/register", `{"email":"someone@example.com"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing fields status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("missing fields error = %q, want invalid_request", got)
	}
}

func TestRegisterWhileLoggedInSwitchesSession(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	ts := newTestApp(t, db)
	client := testClient(t)

	firstEmail := "first+" + tag + "@example.com"
	cleanupUserByEmail(t, db, firstEmail)
	first := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: firstEmail, Nickname: "first" + tag, Password: "pass1234"})
	first.Body.Close()
	if first.StatusCode != http.StatusCreated {
		t.Fatalf("first status = %d, want 201", first.StatusCode)
	}

	secondEmail := "second+" + tag + "@example.com"
	cleanupUserByEmail(t, db, secondEmail)
	second := postJSON(t, client, ts.URL+"/api/v1/auth/register", RegisterRequest{Email: secondEmail, Nickname: "second" + tag, Password: "pass1234"})
	second.Body.Close()
	if second.StatusCode != http.StatusCreated {
		t.Fatalf("second status = %d, want 201", second.StatusCode)
	}

	me, err := client.Get(ts.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer me.Body.Close()
	var body UserResponse
	if err := json.NewDecoder(me.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Email != secondEmail {
		t.Fatalf("me email = %q, want %q", body.Email, secondEmail)
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
			resp := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/register", RegisterRequest{Email: email, Nickname: "race" + tag, Password: "pass1234"})
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

	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: oldPassword})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	change := postJSON(t, client, ts.URL+"/api/v1/auth/password", PasswordChangeRequest{CurrentPassword: oldPassword, NewPassword: "new-pass-123"})
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

	oldLogin := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: oldPassword})
	oldLogin.Body.Close()
	if oldLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, want 401", oldLogin.StatusCode)
	}

	newLogin := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: "new-pass-123"})
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
		resp := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("login status = %d, want 200", resp.StatusCode)
		}
		return client
	}

	t.Run("wrong current password", func(t *testing.T) {
		resp := postJSON(t, loginClient(t), ts.URL+"/api/v1/auth/password", PasswordChangeRequest{CurrentPassword: "wrong-pass-1", NewPassword: "new-pass-123"})
		if resp.StatusCode != http.StatusUnauthorized {
			resp.Body.Close()
			t.Fatalf("status = %d, want 401", resp.StatusCode)
		}
		if got := decodeError(t, resp); got != "invalid_credentials" {
			t.Fatalf("error = %q, want invalid_credentials", got)
		}
	})

	t.Run("weak new password", func(t *testing.T) {
		resp := postJSON(t, loginClient(t), ts.URL+"/api/v1/auth/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "abcdefgh"})
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		if got := decodeError(t, resp); got != "weak_password" {
			t.Fatalf("error = %q, want weak_password", got)
		}
	})

	t.Run("unchanged password", func(t *testing.T) {
		resp := postJSON(t, loginClient(t), ts.URL+"/api/v1/auth/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: password})
		if resp.StatusCode != http.StatusBadRequest {
			resp.Body.Close()
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
		if got := decodeError(t, resp); got != "password_unchanged" {
			t.Fatalf("error = %q, want password_unchanged", got)
		}
	})

	t.Run("anonymous", func(t *testing.T) {
		resp := postJSON(t, testClient(t), ts.URL+"/api/v1/auth/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "new-pass-123"})
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

	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()

	if err := db.Client.User.DeleteOneID(uuid.MustParse(u.ID)).Exec(context.Background()); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	resp := postJSON(t, client, ts.URL+"/api/v1/auth/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "new-pass-123"})
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
		login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
		login.Body.Close()
		if login.StatusCode != http.StatusOK {
			t.Fatalf("login status = %d, want 200", login.StatusCode)
		}
	}

	change := postJSON(t, clientA, ts.URL+"/api/v1/auth/password", PasswordChangeRequest{CurrentPassword: password, NewPassword: "new-pass-123"})
	change.Body.Close()
	if change.StatusCode != http.StatusOK {
		t.Fatalf("change status = %d, want 200", change.StatusCode)
	}

	meA, err := clientA.Get(ts.URL + "/api/v1/auth/me")
	if err != nil {
		t.Fatalf("me A: %v", err)
	}
	meA.Body.Close()
	if meA.StatusCode != http.StatusOK {
		t.Fatalf("client A status = %d, want 200 (current session must survive)", meA.StatusCode)
	}

	for attempt := 1; attempt <= 2; attempt++ {
		meB, err := clientB.Get(ts.URL + "/api/v1/auth/me")
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

	login := postJSON(t, client, ts.URL+"/api/v1/auth/login", LoginRequest{Identifier: u.Email, Password: password})
	login.Body.Close()
	if login.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", login.StatusCode)
	}

	resp := postRaw(t, client, ts.URL+"/api/v1/auth/password", "{not json")
	if resp.StatusCode != http.StatusBadRequest {
		resp.Body.Close()
		t.Fatalf("broken json status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("broken json error = %q, want invalid_request", got)
	}

	resp = postRaw(t, client, ts.URL+"/api/v1/auth/password", `{"current_password":"`+password+`"}`)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing fields status = %d, want 400", resp.StatusCode)
	}
	if got := decodeError(t, resp); got != "invalid_request" {
		t.Fatalf("missing fields error = %q, want invalid_request", got)
	}
}
