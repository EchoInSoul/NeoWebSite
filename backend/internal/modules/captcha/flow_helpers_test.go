package captcha_test

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

func userCount(t *testing.T, db *database.Database) int {
	t.Helper()

	count, err := db.Client.User.Query().Count(context.Background())
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	return count
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
	module := auth.New(db, sessionService, settingsService, loginService, registrationService, twoFactorService, passwordreset.New(db, settingsService, limiter, mailer))
	module.Register(api)
	accessModule := access.New(db)
	protected := api.Group("", guard.RequireAuth(sm), guard.TrackSessionActivity(sm), guard.LoadIdentity(accessModule.Service, sm))
	module.RegisterProtected(protected)

	ts := httptest.NewServer(sm.LoadAndSave(engine))
	t.Cleanup(ts.Close)
	return ts, sm, settingsService
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
