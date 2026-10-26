package captcha

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/common/rbac"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/modules/access"
	"website/backend/internal/modules/captcha/builtin"
	"website/backend/internal/modules/captcha/turnstile"
	"website/backend/internal/modules/settings"
)

type fakeVerifier struct {
	outcome turnstile.Outcome
	calls   int
	secret  string
	token   string
	ip      string
}

func (f *fakeVerifier) Verify(_ context.Context, secret, token, remoteIP string) turnstile.Outcome {
	f.calls++
	f.secret = secret
	f.token = token
	f.ip = remoteIP
	return f.outcome
}

func testDB(t *testing.T) *database.Database {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}

	db, err := database.Open(context.Background(), config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func uniqueTag(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func newCaptchaRouteTestApp(t *testing.T, db *database.Database, verifier *fakeVerifier) (*httptest.Server, *settings.Service) {
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
	engine.GET("/seed", func(c *gin.Context) {
		sm.Put(c.Request.Context(), guard.SessionUserIDKey, c.Query("user"))
		c.Status(http.StatusNoContent)
	})
	api := engine.Group("/api/v1")

	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.LoadIdentity(accessModule.Service, sm))
	captchaService := NewService(settingsModule.Service, verifier)
	builtinService, err := builtin.New(db, settingsModule.Service, ratelimit.New())
	if err != nil {
		t.Fatalf("builtin.New: %v", err)
	}
	module := &Module{
		Service:             captchaService,
		builtinController:   builtin.NewController(builtinService),
		turnstileController: turnstile.NewController(turnstile.NewService(settingsModule.Service, verifier)),
	}
	module.Register(api)
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, settingsModule.Service
}

func seedUserInGroup(t *testing.T, db *database.Database, tag string, groupID uuid.UUID) string {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("pass-"+tag), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	nickname := "cap" + tag
	created, err := db.Client.User.Create().
		SetEmail("cap+" + tag + "@test.local").
		SetNickname(nickname).
		SetPasswordHash(string(hash)).
		SetEmailVerificationStatus("not_required").
		SetGroupID(groupID).
		Save(context.Background())
	if err != nil {
		if ent.IsConstraintError(err) {
			t.Fatalf("user cap+%s@test.local already exists", tag)
		}
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.User.DeleteOneID(created.ID).Exec(context.Background()) })
	return created.ID.String()
}

func fixedGroup(id uuid.UUID) func(t *testing.T, db *database.Database) uuid.UUID {
	return func(_ *testing.T, _ *database.Database) uuid.UUID {
		return id
	}
}

func captchaEditGroup(t *testing.T, db *database.Database) uuid.UUID {
	return createGroupWithPermissions(t, db, "cap", []string{rbac.PermissionCaptchaView, rbac.PermissionCaptchaEdit})
}

func createGroupWithPermissions(t *testing.T, db *database.Database, label string, permissions []string) uuid.UUID {
	t.Helper()

	ctx := context.Background()
	tag := fmt.Sprintf("%s-%d-%d", label, os.Getpid(), time.Now().UnixNano())
	created, err := db.Client.Group.Create().SetName(tag).Save(ctx)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.Group.DeleteOneID(created.ID).Exec(context.Background())
	})
	for _, permission := range permissions {
		if _, err := db.Client.GroupPermission.Create().
			SetGroupID(created.ID).
			SetPermission(permission).
			Save(ctx); err != nil {
			t.Fatalf("create group permission: %v", err)
		}
	}
	return created.ID
}

func testClient(t *testing.T) *http.Client {
	t.Helper()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("jar: %v", err)
	}
	return &http.Client{Jar: jar}
}

func seedSession(t *testing.T, client *http.Client, ts *httptest.Server, userID string) {
	t.Helper()

	resp, err := client.Get(ts.URL + "/seed?user=" + url.QueryEscape(userID))
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("seed session status = %d, want 204", resp.StatusCode)
	}
}

func postJSON(t *testing.T, client *http.Client, endpoint string, body any) *http.Response {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	return resp
}

func postRawWithHeaders(t *testing.T, client *http.Client, endpoint, body string, headers map[string]string) *http.Response {
	t.Helper()

	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return resp
}

func decodeVerifyError(t *testing.T, resp *http.Response) (string, *string) {
	t.Helper()
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var body struct {
		Error struct {
			Code  string  `json:"code"`
			Param *string `json:"param"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode error body %q: %v", raw, err)
	}
	return body.Error.Code, body.Error.Param
}

func TestVerifyCaptchaConfigRoute(t *testing.T) {
	cases := []struct {
		name     string
		outcome  turnstile.Outcome
		groupID  func(t *testing.T, db *database.Database) uuid.UUID
		body     map[string]any
		wantCode int
		wantErr  string
		wantOK   bool
	}{
		{name: "passes", outcome: turnstile.Passed, groupID: fixedGroup(rbac.GroupIDOwner), body: map[string]any{"secret_key": "the-secret", "token": "the-token"}, wantCode: http.StatusOK, wantOK: true},
		{name: "rejection", outcome: turnstile.Rejected, groupID: fixedGroup(rbac.GroupIDOwner), body: map[string]any{"secret_key": "wrong-secret", "token": "the-token"}, wantCode: http.StatusBadRequest, wantErr: "captcha_verification_failed"},
		{name: "unavailable", outcome: turnstile.Unavailable, groupID: fixedGroup(rbac.GroupIDOwner), body: map[string]any{"secret_key": "the-secret", "token": "the-token"}, wantCode: http.StatusServiceUnavailable, wantErr: "service_unavailable"},
		{name: "missing keys", outcome: turnstile.Passed, groupID: fixedGroup(rbac.GroupIDOwner), body: map[string]any{}, wantCode: http.StatusBadRequest, wantErr: "invalid_request"},
		{name: "plain user forbidden", outcome: turnstile.Passed, groupID: fixedGroup(rbac.GroupIDUser), body: map[string]any{"secret_key": "the-secret", "token": "the-token"}, wantCode: http.StatusForbidden, wantErr: "forbidden"},
		{name: "captcha-edit group passes", outcome: turnstile.Passed, groupID: captchaEditGroup, body: map[string]any{"secret_key": "the-secret", "token": "the-token"}, wantCode: http.StatusOK, wantOK: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := testDB(t)
			verifier := &fakeVerifier{outcome: tc.outcome}
			ts, _ := newCaptchaRouteTestApp(t, db, verifier)
			userID := seedUserInGroup(t, db, uniqueTag(t), tc.groupID(t, db))

			client := testClient(t)
			seedSession(t, client, ts, userID)

			resp := postJSON(t, client, ts.URL+"/api/v1/admin/captcha/verifications", tc.body)
			if resp.StatusCode != tc.wantCode {
				t.Fatalf("%s: status = %d, want %d", tc.name, resp.StatusCode, tc.wantCode)
			}
			if tc.wantErr != "" {
				code, _ := decodeVerifyError(t, resp)
				if code != tc.wantErr {
					t.Fatalf("%s: code = %q, want %q", tc.name, code, tc.wantErr)
				}
			}
			if tc.wantOK {
				var body turnstile.VerificationResponse
				if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
					t.Fatalf("decode body: %v", err)
				}
				if !body.Verified {
					t.Fatal("verified = false, want true")
				}
				if verifier.calls != 1 || verifier.secret != "the-secret" || verifier.token != "the-token" {
					t.Fatalf("verifier saw calls=%d secret=%q token=%q", verifier.calls, verifier.secret, verifier.token)
				}
				if verifier.ip == "" {
					t.Fatal("verifier ip is empty, want the request address")
				}
			}
		})
	}
}

func TestVerifyCaptchaConfigRouteRequiresJSONRequest(t *testing.T) {
	db := testDB(t)
	verifier := &fakeVerifier{outcome: turnstile.Passed}
	ts, _ := newCaptchaRouteTestApp(t, db, verifier)
	tag := uniqueTag(t)
	userID := seedUserInGroup(t, db, tag, rbac.GroupIDOwner)

	client := testClient(t)
	seedSession(t, client, ts, userID)

	payload := `{"secret_key":"the-secret","token":"the-token"}`
	cases := []map[string]string{
		{"Content-Type": "text/plain"},
		{"Content-Type": "application/json", "Origin": "https://evil.example"},
	}
	for _, headers := range cases {
		resp := postRawWithHeaders(t, client, ts.URL+"/api/v1/admin/captcha/verifications", payload, headers)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status for %v = %d, want %d", headers, resp.StatusCode, http.StatusBadRequest)
		}
		if code, _ := decodeVerifyError(t, resp); code != "invalid_request" {
			t.Fatalf("code for %v = %q, want invalid_request", headers, code)
		}
	}

	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}

func TestVerifyCaptchaConfigRouteRequiresAuth(t *testing.T) {
	db := testDB(t)
	verifier := &fakeVerifier{outcome: turnstile.Passed}
	ts, _ := newCaptchaRouteTestApp(t, db, verifier)

	resp := postJSON(t, &http.Client{}, ts.URL+"/api/v1/admin/captcha/verifications", map[string]any{"secret_key": "the-secret", "token": "the-token"})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.StatusCode, http.StatusUnauthorized)
	}
	if code, _ := decodeVerifyError(t, resp); code != "unauthorized" {
		t.Fatalf("code = %q, want unauthorized", code)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls = %d, want 0", verifier.calls)
	}
}
