package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/modules/access"
	"website/backend/internal/modules/auth"
	"website/backend/internal/modules/captcha"
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/passwordreset"
	"website/backend/internal/modules/registration"
	"website/backend/internal/modules/session"
	"website/backend/internal/modules/settings"
	"website/backend/internal/modules/twofactor"
)

var testTOTPKey = []byte("0123456789abcdef0123456789abcdef")

type rateLimitPolicies struct {
	LoginIdentifier     ratelimit.FailurePolicy
	LoginIP             ratelimit.WindowPolicy
	RegisterEmail       ratelimit.WindowPolicy
	RegisterIP          ratelimit.WindowPolicy
	EmailVerificationIP ratelimit.WindowPolicy
}

func disabledRateLimitPolicies() rateLimitPolicies {
	return rateLimitPolicies{}
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

func createTestUser(t *testing.T, db *database.Database, tag string) (auth.User, string) {
	t.Helper()

	password := "pass-" + tag
	u, err := auth.CreateAdminUser(context.Background(), db, "Admin+"+tag+"@Example.COM", "nick"+tag, password)
	if err != nil {
		t.Fatalf("create admin user: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(uuid.MustParse(u.ID)).Exec(context.Background())
	})
	return u, password
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

func decodeJSON[T any](t *testing.T, resp *http.Response, want int) T {
	t.Helper()
	defer resp.Body.Close()

	if resp.StatusCode != want {
		t.Fatalf("status = %d, want %d", resp.StatusCode, want)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %T: %v", out, err)
	}
	return out
}

func loginRaw(t *testing.T, client *http.Client, tsURL, identifier, password string) *http.Response {
	t.Helper()

	return postJSON(t, client, tsURL+"/api/v1/sessions", auth.LoginRequest{Identifier: identifier, Password: password})
}

func loginOK(t *testing.T, client *http.Client, tsURL, identifier, password string) {
	t.Helper()

	resp := loginRaw(t, client, tsURL, identifier, password)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
}

func assertErrorCode(t *testing.T, resp *http.Response, wantStatus int, wantCode string) {
	t.Helper()

	if resp.StatusCode != wantStatus {
		resp.Body.Close()
		t.Fatalf("status = %d, want %d", resp.StatusCode, wantStatus)
	}
	if code := decodeError(t, resp); code != wantCode {
		t.Fatalf("code = %q, want %q", code, wantCode)
	}
}

func post2FA(t *testing.T, client *http.Client, tsURL, path string, body any) *http.Response {
	t.Helper()

	return postJSON(t, client, tsURL+"/api/v1/users/me/2fa/"+path, body)
}

func setup2FA(t *testing.T, client *http.Client, tsURL, password string) twofactor.TwoFactorSetupResponse {
	t.Helper()

	resp := post2FA(t, client, tsURL, "setup", twofactor.TwoFactorSetupRequest{Password: password})
	return decodeJSON[twofactor.TwoFactorSetupResponse](t, resp, http.StatusOK)
}

func enable2FA(t *testing.T, client *http.Client, tsURL, secret, password string) twofactor.RecoveryCodesResponse {
	t.Helper()

	resp := post2FA(t, client, tsURL, "enable", twofactor.TwoFactorEnableRequest{Code: twofactor.CodeAt(t, secret, twofactor.CurrentWindow()), Password: password})
	return decodeJSON[twofactor.RecoveryCodesResponse](t, resp, http.StatusOK)
}

func newTestApp(t *testing.T, db *database.Database) *httptest.Server {
	t.Helper()

	ts, _, _ := newTestAppWithSessionsAndSettings(t, db)
	return ts
}

func newTestAppWithSessions(t *testing.T, db *database.Database) (*httptest.Server, *scs.SessionManager) {
	t.Helper()

	ts, sm, _ := newTestAppWithSessionsAndSettings(t, db)
	return ts, sm
}

func newTestAppWithSessionsAndSettings(t *testing.T, db *database.Database) (*httptest.Server, *scs.SessionManager, *settings.Service) {
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
	loginService := login.NewWithPolicies(db, sessionService, ratelimit.New(), login.Policies{}, captchaService)
	registrationService := registration.NewWithPolicies(db, settingsModule.Service, ratelimit.New(), registration.Policies{}, &fakeMailer{configured: true}, captchaService)
	twoFactorService := twofactor.New(db, sessionService, settingsModule.Service, ratelimit.New(), testTOTPKey)
	module := auth.New(db, sessionService, settingsModule.Service, loginService, registrationService, twoFactorService, passwordreset.New(db, settingsModule.Service, ratelimit.New(), &fakeMailer{configured: true}))
	module.Register(api)
	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.TrackSessionActivity(sm), guard.LoadIdentity(accessModule.Service, sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, sm, settingsModule.Service
}

type fakeMailer struct {
	mu            sync.Mutex
	configured    bool
	configuredErr error
	sendSyncErr   error
	messages      []mail.Message
}

func (f *fakeMailer) Configured(context.Context) (bool, error) {
	return f.configured, f.configuredErr
}

func (f *fakeMailer) Send(context.Context, mail.Message) error {
	return nil
}

func (f *fakeMailer) SendSync(_ context.Context, msg mail.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.sendSyncErr != nil {
		return f.sendSyncErr
	}
	f.messages = append(f.messages, msg)
	return nil
}

func (f *fakeMailer) Close(context.Context) error {
	return nil
}
