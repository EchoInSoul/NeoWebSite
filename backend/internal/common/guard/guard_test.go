package guard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/seed", func(c *gin.Context) {
		sm.Put(c.Request.Context(), SessionUserIDKey, "u1")
		c.Status(http.StatusNoContent)
	})
	r.GET("/protected", RequireAuth(sm), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user": c.GetString(SessionUserIDKey)})
	})

	ts := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(ts.Close)
	return ts
}

func TestRequireAuthRejectsAnonymous(t *testing.T) {
	ts := newTestServer(t)

	resp, err := http.Get(ts.URL + "/protected")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if code := decodeErrorBody(t, resp); code != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", code)
	}
}

func TestRequireAuthAllowsSessionUser(t *testing.T) {
	ts := newTestServer(t)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	client := &http.Client{Jar: jar}

	seed, err := client.Get(ts.URL + "/seed")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	seed.Body.Close()

	resp, err := client.Get(ts.URL + "/protected")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["user"] != "u1" {
		t.Fatalf("user = %q, want u1", body["user"])
	}
}

type fakeResolver struct {
	identities map[string]Identity
	err        error
}

func (f fakeResolver) IdentityByUserID(_ context.Context, userID string) (Identity, error) {
	if f.err != nil {
		return Identity{}, f.err
	}
	identity, ok := f.identities[userID]
	if !ok {
		return Identity{}, ErrUserNotFound
	}
	return identity, nil
}

func newPermissionTestServer(t *testing.T, resolver IdentityResolver) *httptest.Server {
	t.Helper()

	sm := scs.New()
	sm.Lifetime = time.Hour

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/seed", func(c *gin.Context) {
		sm.Put(c.Request.Context(), SessionUserIDKey, "u1")
		c.Status(http.StatusNoContent)
	})
	r.GET("/identity-unauthed", LoadIdentity(resolver, sm), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	r.GET("/identity", RequireAuth(sm), LoadIdentity(resolver, sm), func(c *gin.Context) {
		identity, ok := IdentityFrom(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "identity_missing"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"user":        identity.UserID,
			"group":       identity.GroupKey,
			"permissions": identity.Permissions,
		})
	})
	r.GET("/gated", RequireAuth(sm), LoadIdentity(resolver, sm), RequirePermission("system.users.edit"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	r.GET("/gated-unwired", RequirePermission("system.users.edit"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	ts := httptest.NewServer(sm.LoadAndSave(r))
	t.Cleanup(ts.Close)
	return ts
}

func seededClient(t *testing.T, ts *httptest.Server) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	client := &http.Client{Jar: jar}
	resp, err := client.Get(ts.URL + "/seed")
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	resp.Body.Close()
	return client
}

func decodeErrorBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want nested object with code", body["error"])
	}
	code, _ := errObj["code"].(string)
	return code
}

func TestLoadIdentityRejectsSessionlessRequest(t *testing.T) {
	ts := newPermissionTestServer(t, fakeResolver{identities: map[string]Identity{
		"u1": {UserID: "u1"},
	}})

	resp, err := http.Get(ts.URL + "/identity-unauthed")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if code := decodeErrorBody(t, resp); code != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", code)
	}
}

func TestLoadIdentityDestroysSessionWhenUserIsGone(t *testing.T) {
	ts := newPermissionTestServer(t, fakeResolver{identities: map[string]Identity{}})
	client := seededClient(t, ts)

	resp, err := client.Get(ts.URL + "/identity")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	if code := decodeErrorBody(t, resp); code != "unauthorized" {
		t.Fatalf("error = %q, want unauthorized", code)
	}

	cleared := false
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "session" && cookie.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatalf("session cookie was not cleared; cookies: %v", resp.Cookies())
	}
}

func TestLoadIdentityFailsOnResolverError(t *testing.T) {
	ts := newPermissionTestServer(t, fakeResolver{err: errors.New("boom")})
	client := seededClient(t, ts)

	resp, err := client.Get(ts.URL + "/identity")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if code := decodeErrorBody(t, resp); code != "internal_error" {
		t.Fatalf("error = %q, want internal_error", code)
	}
}

func TestRequirePermissionWithoutIdentity(t *testing.T) {
	ts := newPermissionTestServer(t, fakeResolver{identities: map[string]Identity{}})

	resp, err := http.Get(ts.URL + "/gated-unwired")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if code := decodeErrorBody(t, resp); code != "internal_error" {
		t.Fatalf("error = %q, want internal_error", code)
	}
}

func TestLoadIdentityExposesIdentityAndGatesPermissions(t *testing.T) {
	ts := newPermissionTestServer(t, fakeResolver{identities: map[string]Identity{
		"u1": {
			UserID:      "u1",
			GroupID:     "g1",
			GroupKey:    "admin",
			GroupName:   "管理员",
			Permissions: []string{"system.users.edit"},
		},
	}})
	client := seededClient(t, ts)

	resp, err := client.Get(ts.URL + "/identity")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("identity status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		User        string   `json:"user"`
		Group       string   `json:"group"`
		Permissions []string `json:"permissions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.User != "u1" || body.Group != "admin" {
		t.Fatalf("identity = %+v, want u1/admin", body)
	}
	if len(body.Permissions) != 1 || body.Permissions[0] != "system.users.edit" {
		t.Fatalf("permissions = %v, want [system.users.edit]", body.Permissions)
	}

	gated, err := client.Get(ts.URL + "/gated")
	if err != nil {
		t.Fatalf("get gated: %v", err)
	}
	defer gated.Body.Close()
	if gated.StatusCode != http.StatusNoContent {
		t.Fatalf("gated status = %d, want 204", gated.StatusCode)
	}

	tsDenied := newPermissionTestServer(t, fakeResolver{identities: map[string]Identity{
		"u1": {UserID: "u1", GroupKey: "user", Permissions: []string{"system.users.view"}},
	}})
	clientDenied := seededClient(t, tsDenied)

	denied, err := clientDenied.Get(tsDenied.URL + "/gated")
	if err != nil {
		t.Fatalf("get denied: %v", err)
	}
	defer denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("denied status = %d, want 403", denied.StatusCode)
	}
	if code := decodeErrorBody(t, denied); code != "forbidden" {
		t.Fatalf("error = %q, want forbidden", code)
	}
}
