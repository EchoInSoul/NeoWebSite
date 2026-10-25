package registration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/user"
	"website/backend/internal/modules/access"
	"website/backend/internal/modules/auth"
	"website/backend/internal/modules/captcha"
	"website/backend/internal/modules/login"
	"website/backend/internal/modules/mail"
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

func defaultRateLimitPolicies() rateLimitPolicies {
	loginPolicies := login.DefaultPolicies()
	registrationPolicies := registration.DefaultPolicies()
	return rateLimitPolicies{
		LoginIdentifier:     loginPolicies.IdentifierFailure,
		LoginIP:             loginPolicies.IPWindow,
		RegisterEmail:       registrationPolicies.EmailWindow,
		RegisterIP:          registrationPolicies.IPWindow,
		EmailVerificationIP: registrationPolicies.EmailVerificationWindow,
	}
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

func cleanupUserByEmail(t *testing.T, db *database.Database, email string) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background())
	})
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

func loginAs(t *testing.T, tsURL, email, password string) *http.Client {
	t.Helper()

	client := testClient(t)
	resp := postJSON(t, client, tsURL+"/api/v1/sessions", auth.LoginRequest{Identifier: email, Password: password})
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login status = %d, want 200", resp.StatusCode)
	}
	return client
}

func newTestApp(t *testing.T, db *database.Database) *httptest.Server {
	t.Helper()

	ts, _, _ := newTestAppWithSessionsAndSettings(t, db)
	return ts
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

	return newTestAppWithCaptchaVerifier(t, db, settingsService, ratelimit.New(), disabledRateLimitPolicies(), nil, &fakeMailer{configured: true})
}

func newTestAppWithMailer(t *testing.T, db *database.Database, mailer mail.Mailer) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return newTestAppWithCaptchaVerifier(t, db, settingsModule.Service, ratelimit.New(), disabledRateLimitPolicies(), nil, mailer)
}

func newTestAppWithRatePolicies(t *testing.T, db *database.Database, policies rateLimitPolicies) (*httptest.Server, *scs.SessionManager, *settings.Service) {
	t.Helper()

	settingsModule, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return newTestAppWithCaptchaVerifier(t, db, settingsModule.Service, ratelimit.New(), policies, nil, &fakeMailer{configured: true})
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
	loginService := login.NewWithPolicies(db, sessionService, limiter, login.Policies{
		IdentifierFailure: policies.LoginIdentifier,
		IPWindow:          policies.LoginIP,
	}, captchaService)
	registrationService := registration.NewWithPolicies(db, settingsService, limiter, registration.Policies{
		EmailWindow:             policies.RegisterEmail,
		IPWindow:                policies.RegisterIP,
		EmailVerificationWindow: policies.EmailVerificationIP,
	}, mailer, captchaService)
	twoFactorService := twofactor.New(db, sessionService, settingsService, ratelimit.New(), testTOTPKey)
	module := auth.New(db, sessionService, settingsService, loginService, registrationService, twoFactorService)
	module.Register(api)
	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.TrackSessionActivity(sm), guard.LoadIdentity(accessModule.Service, sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, sm, settingsService
}
