package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

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

	ts := httptest.NewServer(New(cfg, db).Handler())
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
	ts, db := newTestServer(t, "development")
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
	ts, db := newTestServer(t, "development")
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
	ts, db := newTestServer(t, "production")
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

func TestSessionCookieMaxAgeMatchesLifetime(t *testing.T) {
	ts, db := newTestServer(t, "development")
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
	ts, _ := newTestServer(t, "development")
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
	ts, _ := newTestServer(t, "development")

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

func TestHealthyEndpointStillWorks(t *testing.T) {
	ts, _ := newTestServer(t, "development")

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

func TestChangePasswordToleratesBadSessionRows(t *testing.T) {
	ts, db := newTestServer(t, "development")
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
	cfg := testConfig(t, "development")
	db, err := database.Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	a := New(cfg, db)
	if err := a.shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	err = a.mailer.Send(context.Background(), mail.Message{To: "user@example.com", Subject: "s", Body: mail.Body{Text: "t"}})
	if !errors.Is(err, mail.ErrClosed) {
		t.Fatalf("send after shutdown = %v, want mail.ErrClosed", err)
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
	ts, db := newTestServer(t, "development")
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
