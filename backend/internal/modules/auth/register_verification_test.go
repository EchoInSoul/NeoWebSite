package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/pendingregistration"
	"website/backend/internal/database/ent/user"
	"website/backend/internal/modules/mail"
	"website/backend/internal/modules/settings"
)

func testPendingToken(t *testing.T) (string, string) {
	t.Helper()

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("read token bytes: %v", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(buf)
	return raw, testHashPendingToken(raw)
}

func testHashPendingToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

var errTestSend = errors.New("test send failure")

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

func (f *fakeMailer) sent() []mail.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]mail.Message(nil), f.messages...)
}

func (f *fakeMailer) Close(context.Context) error {
	return nil
}

func enableEmailVerification(t *testing.T, svc *settings.Service) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetBool(ctx, "register", "email_verification_enabled", true); err != nil {
		t.Fatalf("set email_verification_enabled: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "register", "email_verification_enabled"); err != nil {
			t.Errorf("reset email_verification_enabled: %v", err)
		}
	})
}

func setSiteURL(t *testing.T, svc *settings.Service, value string) {
	t.Helper()
	ctx := context.Background()
	if err := svc.SetString(ctx, "site", "url", value); err != nil {
		t.Fatalf("set site.url: %v", err)
	}
	t.Cleanup(func() {
		if err := svc.ResetItem(ctx, "site", "url"); err != nil {
			t.Errorf("reset site.url: %v", err)
		}
	})
}

func pendingCount(t *testing.T, db *database.Database, email string) int {
	t.Helper()
	count, err := db.Client.PendingRegistration.Query().
		Where(pendingregistration.EmailEQ(email)).
		Count(context.Background())
	if err != nil {
		t.Fatalf("count pending: %v", err)
	}
	return count
}

func extractToken(t *testing.T, body string) string {
	t.Helper()
	match := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("no verification token in %q", body)
	}
	return match[1]
}

func postJSONStatus(client *http.Client, endpoint string, body any) (int, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, err
	}
	resp, err := client.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	return resp.StatusCode, nil
}

func TestRegisterCreatesPendingInsteadOfUser(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")

	before := userCount(t, db)
	email := "verify+" + tag + "@example.com"
	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "v" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", resp.StatusCode)
	}
	rawBody, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(rawBody), email) {
		t.Fatalf("202 leaked email: %s", rawBody)
	}
	var body map[string]any
	_ = json.Unmarshal(rawBody, &body)
	if len(body) != 1 || body["status"] != "verification_pending" {
		t.Fatalf("body = %+v", body)
	}
	if after := userCount(t, db); after != before {
		t.Fatalf("users = %d, want %d", after, before)
	}
	if pendingCount(t, db, email) != 1 {
		t.Fatal("want one pending registration")
	}
	row, err := db.Client.PendingRegistration.Query().Where(pendingregistration.EmailEQ(email)).Only(context.Background())
	if err != nil {
		t.Fatalf("find pending: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(row.PasswordHash), []byte("pass1234")) != nil {
		t.Fatal("pending password hash does not verify")
	}
	if len(row.TokenHash) != 64 {
		t.Fatalf("token hash length = %d, want 64", len(row.TokenHash))
	}
	if row.ExpiresAt.Before(time.Now().Add(29*time.Minute)) || row.ExpiresAt.After(time.Now().Add(31*time.Minute)) {
		t.Fatalf("expires_at = %v, want ~30m", row.ExpiresAt)
	}
	mails := mailer.sent()
	if len(mails) != 1 {
		t.Fatalf("mails = %d, want 1", len(mails))
	}
	token := extractToken(t, mails[0].Body.Text)
	if row.TokenHash != testHashPendingToken(token) {
		t.Fatal("stored token hash does not match mailed token")
	}
	if !mails[0].RequireSMTP {
		t.Fatal("verification mail must require SMTP")
	}
}

func TestRegisterVerificationOffCreatesNotRequiredUser(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, _ := newTestAppWithMailer(t, db, mailer)
	email := "off+" + tag + "@example.com"
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background())
	})

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "off" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	row, err := db.Client.User.Query().Where(user.EmailEQ(email)).Only(context.Background())
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if row.EmailVerificationStatus != "not_required" {
		t.Fatalf("status = %q, want not_required", row.EmailVerificationStatus)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("verification-off register created a pending row")
	}
	if len(mailer.sent()) != 0 {
		t.Fatal("verification-off register sent mail")
	}
}

func TestRegisterWithoutSMTPFailsClosed(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: false}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "nosmtp+" + tag + "@example.com"
	before := userCount(t, db)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "ns" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if code := decodeError(t, resp); code != "mail_unavailable" {
		t.Fatalf("code = %q, want mail_unavailable", code)
	}
	if after := userCount(t, db); after != before {
		t.Fatalf("users = %d, want %d", after, before)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("unconfigured SMTP left a pending row")
	}
	if len(mailer.sent()) != 0 {
		t.Fatal("unconfigured SMTP sent mail")
	}
}

func TestRegisterWithoutSiteURLFailsClosed(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "")
	email := "nosite+" + tag + "@example.com"

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "nsite" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("missing site.url left a pending row")
	}
}

func TestRegisterInvalidSiteURL(t *testing.T) {
	cases := []string{
		"example.com",
		"ftp://example.com",
		"javascript:alert(1)",
		"http://:8080",
		"https://example.com/?a=1",
		"https://example.com/#x",
	}
	for _, siteURL := range cases {
		t.Run(siteURL, func(t *testing.T) {
			db := testDB(t)
			tag := uniqueTag(t)
			mailer := &fakeMailer{configured: true}
			ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
			enableEmailVerification(t, settingsService)
			setSiteURL(t, settingsService, siteURL)
			email := "badurl+" + tag + "@example.com"

			resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
				RegisterRequest{Email: email, Nickname: "bu" + tag, Password: "pass1234"})
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503 for %q", resp.StatusCode, siteURL)
			}
			if pendingCount(t, db, email) != 0 {
				t.Fatal("invalid site.url left a pending row")
			}
		})
	}
}

func TestRegisterBuildsVerifyLinkWithSitePath(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com/blog/")
	email := "path+" + tag + "@example.com"

	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "p" + tag, Password: "pass1234"}).Body.Close()
	mails := mailer.sent()
	if len(mails) != 1 {
		t.Fatalf("mails = %d, want 1", len(mails))
	}
	if !strings.Contains(mails[0].Body.Text, "https://example.com/blog/register/verify?token=") {
		t.Fatalf("body = %q", mails[0].Body.Text)
	}
}

func TestRegisterSendFailureDeletesPending(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true, sendSyncErr: errTestSend}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "sendfail+" + tag + "@example.com"
	before := userCount(t, db)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "sf" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	if after := userCount(t, db); after != before {
		t.Fatalf("users = %d, want %d", after, before)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("send failure left a pending row")
	}
}

func TestRegisterResubmitReplacesPending(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "resubmit+" + tag + "@example.com"

	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "rs" + tag, Password: "pass1234"}).Body.Close()
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "rs" + tag, Password: "pass1234"}).Body.Close()

	if pendingCount(t, db, email) != 1 {
		t.Fatal("resubmit must keep exactly one pending row")
	}
	mails := mailer.sent()
	if len(mails) != 2 {
		t.Fatalf("mails = %d, want 2", len(mails))
	}
	firstToken := extractToken(t, mails[0].Body.Text)
	secondToken := extractToken(t, mails[1].Body.Text)
	if firstToken == secondToken {
		t.Fatal("resubmit reused the old token")
	}
	row, err := db.Client.PendingRegistration.Query().Where(pendingregistration.EmailEQ(email)).Only(context.Background())
	if err != nil {
		t.Fatalf("find pending: %v", err)
	}
	if row.TokenHash != testHashPendingToken(secondToken) {
		t.Fatal("stored token hash is not the second token")
	}
}

func TestRegisterVerificationConcurrentSameEmail(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "race+" + tag + "@example.com"

	statuses := make([]int, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], errs[i] = postJSONStatus(testClient(t), ts.URL+"/api/v1/users",
				RegisterRequest{Email: email, Nickname: "race" + tag, Password: "pass1234"})
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
		if statuses[i] != http.StatusAccepted {
			t.Fatalf("statuses = %v, want two 202", statuses)
		}
	}
	if pendingCount(t, db, email) != 1 {
		t.Fatalf("pending = %d, want 1", pendingCount(t, db, email))
	}
}

func TestRegisterWithPendingAndSwitchOffCreatesUser(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "switchoff+" + tag + "@example.com"

	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "so" + tag, Password: "pass1234"}).Body.Close()
	if pendingCount(t, db, email) != 1 {
		t.Fatal("setup did not create a pending row")
	}
	if err := settingsService.SetBool(context.Background(), "register", "email_verification_enabled", false); err != nil {
		t.Fatalf("disable verification: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background())
	})

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "so" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	row, err := db.Client.User.Query().Where(user.EmailEQ(email)).Only(context.Background())
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if row.EmailVerificationStatus != "not_required" {
		t.Fatalf("status = %q, want not_required", row.EmailVerificationStatus)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("switch-off register left the pending row")
	}
}

func TestRegisterVerificationEnabledRejectsExistingEmail(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "takenemail+" + tag + "@example.com"
	_, err := db.Client.User.Create().
		SetEmail(email).SetNickname("te" + tag).SetPasswordHash("hash").
		SetEmailVerificationStatus("not_required").Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background())
	})

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "te" + tag, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("taken email left a pending row")
	}
	if len(mailer.sent()) != 0 {
		t.Fatal("taken email sent mail")
	}
}

func TestRegisterVerificationEnabledRejectsExistingNickname(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	nickname := "tn" + tag
	_, err := db.Client.User.Create().
		SetEmail("takenuser+" + tag + "@example.com").SetNickname(nickname).SetPasswordHash("hash").
		SetEmailVerificationStatus("not_required").Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.NicknameEQ(nickname)).Exec(context.Background())
	})

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: "newemail+" + tag + "@example.com", Nickname: nickname, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	if len(mailer.sent()) != 0 {
		t.Fatal("taken nickname sent mail")
	}
}

func TestRegisterAllowsSameNicknameAcrossPending(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	nickname := "same" + tag

	for _, email := range []string{"same1+" + tag + "@example.com", "same2+" + tag + "@example.com"} {
		t.Cleanup(func() {
			_, _ = db.Client.PendingRegistration.Delete().
				Where(pendingregistration.EmailEQ(email)).Exec(context.Background())
		})
		resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users",
			RegisterRequest{Email: email, Nickname: nickname, Password: "pass1234"})
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("%s: status = %d, want 202", email, resp.StatusCode)
		}
	}
	for _, email := range []string{"same1+" + tag + "@example.com", "same2+" + tag + "@example.com"} {
		if pendingCount(t, db, email) != 1 {
			t.Fatalf("%s: want one pending row", email)
		}
	}
}

func TestRegisterDoesNotUseLogSender(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "requiresmtp+" + tag + "@example.com"

	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "rsmtp" + tag, Password: "pass1234"}).Body.Close()
	mails := mailer.sent()
	if len(mails) != 1 {
		t.Fatalf("mails = %d, want 1", len(mails))
	}
	if !mails[0].RequireSMTP {
		t.Fatal("verification mail must set RequireSMTP")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	return parsed
}

func TestVerifyEmailRegistration(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "verifyok+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "vo" + tag, Password: "pass1234"}).Body.Close()
	token := extractToken(t, mailer.sent()[0].Body.Text)

	client := testClient(t)
	resp := postJSON(t, client, ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	row, err := db.Client.User.Query().Where(user.EmailEQ(email)).Only(context.Background())
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if row.EmailVerificationStatus != "verified" {
		t.Fatalf("status = %q, want verified", row.EmailVerificationStatus)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("verify left the pending row")
	}
	if len(client.Jar.Cookies(mustURL(t, ts.URL))) != 0 {
		t.Fatal("verify must not create a session")
	}
}

func TestVerifyEmailRegistrationRejectsUnknownAndExpired(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "expired+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "ex" + tag, Password: "pass1234"}).Body.Close()
	token := extractToken(t, mailer.sent()[0].Body.Text)

	unknown := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": "unknown"})
	if unknown.StatusCode != http.StatusBadRequest || decodeError(t, unknown) != "email_verification_invalid" {
		t.Fatalf("unknown status = %d", unknown.StatusCode)
	}

	if _, err := db.Client.PendingRegistration.Update().
		Where(pendingregistration.EmailEQ(email)).
		SetExpiresAt(time.Now().Add(-time.Minute)).
		Save(context.Background()); err != nil {
		t.Fatalf("expire pending: %v", err)
	}
	expired := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
	if expired.StatusCode != http.StatusBadRequest || decodeError(t, expired) != "email_verification_invalid" {
		t.Fatalf("expired status = %d", expired.StatusCode)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("expired pending row was not deleted")
	}
}

func TestVerifyEmailRegistrationConcurrent(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "vrace+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "vr" + tag, Password: "pass1234"}).Body.Close()
	token := extractToken(t, mailer.sent()[0].Body.Text)

	statuses := make([]int, 2)
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range statuses {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			statuses[i], errs[i] = postJSONStatus(testClient(t), ts.URL+"/api/v1/users/email-verifications",
				map[string]string{"token": token})
		}(i)
	}
	wg.Wait()
	for i := range errs {
		if errs[i] != nil {
			t.Fatalf("request %d: %v", i, errs[i])
		}
	}
	if !((statuses[0] == http.StatusCreated && statuses[1] == http.StatusBadRequest) ||
		(statuses[0] == http.StatusBadRequest && statuses[1] == http.StatusCreated)) {
		t.Fatalf("statuses = %v, want one 201 and one 400", statuses)
	}
	count, err := db.Client.User.Query().Where(user.EmailEQ(email)).Count(context.Background())
	if err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("users = %d, want 1", count)
	}
}

func TestVerifyPendingEmailTaken(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "taken+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "tk" + tag, Password: "pass1234"}).Body.Close()
	token := extractToken(t, mailer.sent()[0].Body.Text)
	_, err := db.Client.User.Create().SetEmail(email).SetNickname("other" + tag).SetPasswordHash("hash").
		SetEmailVerificationStatus("not_required").Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(email)).Exec(context.Background())
	})

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || decodeError(t, resp) != "email_taken" {
		t.Fatalf("status = %d, want 409 email_taken", resp.StatusCode)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("email_taken left the pending row")
	}
}

func TestVerifyPendingNicknameTaken(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "nt+" + tag + "@example.com"
	nickname := "nt" + tag
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: nickname, Password: "pass1234"}).Body.Close()
	token := extractToken(t, mailer.sent()[0].Body.Text)
	_, err := db.Client.User.Create().SetEmail("other+" + tag + "@example.com").SetNickname(nickname).SetPasswordHash("hash").
		SetEmailVerificationStatus("not_required").Save(context.Background())
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.NicknameEQ(nickname)).Exec(context.Background())
	})

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict || decodeError(t, resp) != "nickname_taken" {
		t.Fatalf("status = %d, want 409 nickname_taken", resp.StatusCode)
	}
	if pendingCount(t, db, email) != 0 {
		t.Fatal("nickname_taken left the pending row")
	}
}

func TestVerifyConcurrentSameNickname(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	nickname := "cn" + tag
	email1 := "cn1+" + tag + "@example.com"
	email2 := "cn2+" + tag + "@example.com"
	raw1, hash1 := testPendingToken(t)
	raw2, hash2 := testPendingToken(t)
	for _, in := range []struct {
		email string
		hash  string
	}{
		{email: email1, hash: hash1},
		{email: email2, hash: hash2},
	} {
		if _, err := db.Client.PendingRegistration.Create().
			SetEmail(in.email).
			SetNickname(nickname).
			SetPasswordHash("hash").
			SetTokenHash(in.hash).
			SetExpiresAt(time.Now().Add(30 * time.Minute)).
			Save(ctx); err != nil {
			t.Fatalf("seed pending: %v", err)
		}
	}
	dropCollisionTrigger := installNicknameCollisionTrigger(t, db, nickname)
	t.Cleanup(dropCollisionTrigger)
	t.Cleanup(func() {
		_, _ = db.Client.PendingRegistration.Delete().Where(
			pendingregistration.EmailIn(email1, email2)).Exec(context.Background())
		_, _ = db.Client.User.Delete().Where(user.NicknameEQ(nickname)).Exec(context.Background())
	})

	mailer := &fakeMailer{configured: true}
	ts, _, _ := newTestAppWithMailer(t, db, mailer)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": raw1})
	if resp.StatusCode != http.StatusConflict || decodeError(t, resp) != "already_taken" {
		t.Fatalf("triggered verify status = %d, want 409 already_taken", resp.StatusCode)
	}
	if pendingCount(t, db, email1) != 1 {
		t.Fatal("rollback did not restore the pending row")
	}

	dropCollisionTrigger()
	retry := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": raw1})
	if retry.StatusCode != http.StatusCreated {
		t.Fatalf("retry status = %d, want 201", retry.StatusCode)
	}
	second := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": raw2})
	if second.StatusCode != http.StatusConflict || decodeError(t, second) != "nickname_taken" {
		t.Fatalf("second verify status = %d, want 409 nickname_taken", second.StatusCode)
	}
	if pendingCount(t, db, email2) != 0 {
		t.Fatal("loser pending row was not deleted")
	}
}

func installNicknameCollisionTrigger(t *testing.T, db *database.Database, nickname string) func() {
	t.Helper()
	drop := func() {
		_, _ = db.SQL().Exec("DROP TRIGGER IF EXISTS auth_pending_nickname_collision ON users")
		_, _ = db.SQL().Exec("DROP FUNCTION IF EXISTS auth_pending_nickname_collision()")
	}
	drop()
	if _, err := db.SQL().Exec(`
CREATE OR REPLACE FUNCTION auth_pending_nickname_collision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
	IF NEW.nickname = '` + nickname + `' AND NEW.email NOT LIKE '%@collision.test' THEN
		INSERT INTO users (id, email, nickname, password_hash, email_verification_status, group_id, created_at, updated_at)
		VALUES (gen_random_uuid(), NEW.nickname || '@collision.test', NEW.nickname, 'hash', 'not_required', NEW.group_id, now(), now());
	END IF;
	RETURN NEW;
END $$`); err != nil {
		t.Fatalf("create collision function: %v", err)
	}
	if _, err := db.SQL().Exec(`
CREATE TRIGGER auth_pending_nickname_collision BEFORE INSERT ON users
FOR EACH ROW EXECUTE FUNCTION auth_pending_nickname_collision()`); err != nil {
		drop()
		t.Fatalf("create collision trigger: %v", err)
	}
	once := sync.Once{}
	return func() { once.Do(drop) }
}

func TestVerifyOldTokenInvalidAfterResubmit(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "oldtoken+" + tag + "@example.com"

	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "ot" + tag, Password: "pass1234"}).Body.Close()
	first := extractToken(t, mailer.sent()[0].Body.Text)
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "ot" + tag, Password: "pass1234"}).Body.Close()
	second := extractToken(t, mailer.sent()[1].Body.Text)

	old := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": first})
	if old.StatusCode != http.StatusBadRequest {
		t.Fatalf("old token status = %d, want 400", old.StatusCode)
	}
	fresh := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": second})
	if fresh.StatusCode != http.StatusCreated {
		t.Fatalf("new token status = %d, want 201", fresh.StatusCode)
	}
}

func TestVerifyEmailRegistrationAfterSwitchOff(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "afterswitch+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "as" + tag, Password: "pass1234"}).Body.Close()
	token := extractToken(t, mailer.sent()[0].Body.Text)
	if err := settingsService.SetBool(context.Background(), "register", "email_verification_enabled", false); err != nil {
		t.Fatalf("disable verification: %v", err)
	}

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": token})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
}

func TestVerifyRateLimit(t *testing.T) {
	db := testDB(t)
	policies := rateLimitPolicies{EmailVerificationIP: ratelimit.WindowPolicy{Limit: 1, Window: time.Hour}}
	ts, _, _ := newTestAppWithRatePolicies(t, db, policies)

	first := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": "bogus"})
	if first.StatusCode != http.StatusBadRequest {
		t.Fatalf("first status = %d, want 400", first.StatusCode)
	}
	second := postJSON(t, testClient(t), ts.URL+"/api/v1/users/email-verifications", map[string]string{"token": "bogus"})
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.StatusCode)
	}
}

func patchJSON(t *testing.T, client *http.Client, endpoint string, body any) *http.Response {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	req, err := http.NewRequest(http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("patch %s: %v", endpoint, err)
	}
	return resp
}

func TestLoginPendingRegistrationCorrectPassword(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "pend+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "pd" + tag, Password: "pass1234"}).Body.Close()

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: email, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden || decodeError(t, resp) != "email_not_verified" {
		t.Fatalf("status = %d, want 403 email_not_verified", resp.StatusCode)
	}
}

func TestLoginPendingRegistrationWrongPassword(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "pendwrong+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "pw" + tag, Password: "pass1234"}).Body.Close()

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: email, Password: "wrong123"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || decodeError(t, resp) != "invalid_credentials" {
		t.Fatalf("status = %d, want 401 invalid_credentials", resp.StatusCode)
	}
}

func TestLoginPendingRegistrationExpired(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	email := "pendexp+" + tag + "@example.com"
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: email, Nickname: "pe" + tag, Password: "pass1234"}).Body.Close()
	if _, err := db.Client.PendingRegistration.Update().
		Where(pendingregistration.EmailEQ(email)).
		SetExpiresAt(time.Now().Add(-time.Minute)).
		Save(context.Background()); err != nil {
		t.Fatalf("expire pending: %v", err)
	}

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: email, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || decodeError(t, resp) != "invalid_credentials" {
		t.Fatalf("status = %d, want 401 invalid_credentials", resp.StatusCode)
	}
}

func TestLoginPendingRegistrationByNickname(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableEmailVerification(t, settingsService)
	setSiteURL(t, settingsService, "https://example.com")
	nickname := "pn" + tag
	postJSON(t, testClient(t), ts.URL+"/api/v1/users",
		RegisterRequest{Email: "pennick+" + tag + "@example.com", Nickname: nickname, Password: "pass1234"}).Body.Close()

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: nickname, Password: "pass1234"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || decodeError(t, resp) != "invalid_credentials" {
		t.Fatalf("status = %d, want 401 invalid_credentials", resp.StatusCode)
	}
}

func TestLoginPendingRegistrationWrongPasswordCountsFailure(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	email := "pendcount+" + tag + "@example.com"
	hash, err := bcrypt.GenerateFromPassword([]byte("pass1234"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	_, err = db.Client.PendingRegistration.Create().
		SetEmail(email).SetNickname("pc" + tag).SetPasswordHash(string(hash)).
		SetTokenHash("token-" + tag).SetExpiresAt(time.Now().Add(30 * time.Minute)).
		Save(context.Background())
	if err != nil {
		t.Fatalf("seed pending: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.Client.PendingRegistration.Delete().
			Where(pendingregistration.EmailEQ(email)).Exec(context.Background())
	})

	policies := rateLimitPolicies{LoginIdentifier: ratelimit.FailurePolicy{Limit: 1, Window: time.Minute, Cooldown: time.Minute}}
	ts, _, _ := newTestAppWithRatePolicies(t, db, policies)

	first := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: email, Password: "wrong123"})
	if first.StatusCode != http.StatusUnauthorized {
		t.Fatalf("first status = %d, want 401", first.StatusCode)
	}
	second := postJSON(t, testClient(t), ts.URL+"/api/v1/sessions", LoginRequest{Identifier: email, Password: "wrong123"})
	if second.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second status = %d, want 429", second.StatusCode)
	}
}

func TestUpdateProfileEmailKeepsVerificationStatus(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	u, password := createTestUser(t, db, tag)
	ctx := context.Background()
	if _, err := db.Client.User.Update().
		Where(user.EmailEQ(u.Email)).
		SetEmailVerificationStatus("verified").
		Save(ctx); err != nil {
		t.Fatalf("mark verified: %v", err)
	}

	ts := newTestApp(t, db)
	client := loginAs(t, ts.URL, u.Email, password)
	newEmail := "newmail+" + tag + "@example.com"
	resp := patchJSON(t, client, ts.URL+"/api/v1/users/me", map[string]any{"email": newEmail})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("patch status = %d, want 200", resp.StatusCode)
	}
	t.Cleanup(func() {
		_, _ = db.Client.User.Delete().Where(user.EmailEQ(newEmail)).Exec(context.Background())
	})
	row, err := db.Client.User.Query().Where(user.EmailEQ(newEmail)).Only(ctx)
	if err != nil {
		t.Fatalf("find user: %v", err)
	}
	if row.EmailVerificationStatus != "verified" {
		t.Fatalf("status = %q, want verified", row.EmailVerificationStatus)
	}
}
