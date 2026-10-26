package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"testing"

	"website/backend/internal/modules/settings"
)

func enableReset(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetBool(ctx, "auth", "password_reset_enabled", true); err != nil {
		t.Fatalf("enable reset: %v", err)
	}
	t.Cleanup(func() { _ = svc.ResetItem(ctx, "auth", "password_reset_enabled") })
}

func setResetSite(t *testing.T, svc *settings.Service) {
	t.Helper()

	ctx := context.Background()
	if err := svc.SetString(ctx, "site", "title", "My Blog"); err != nil {
		t.Fatalf("set title: %v", err)
	}
	if err := svc.SetString(ctx, "site", "url", "https://example.com"); err != nil {
		t.Fatalf("set url: %v", err)
	}
	t.Cleanup(func() {
		_ = svc.ResetItem(ctx, "site", "title")
		_ = svc.ResetItem(ctx, "site", "url")
	})
}

func decodeErrorFields(t *testing.T, resp *http.Response) (string, string) {
	t.Helper()
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	errObj, _ := body["error"].(map[string]any)
	code, _ := errObj["code"].(string)
	param, _ := errObj["param"].(string)
	return code, param
}

func resetTokenFrom(t *testing.T, mailer *fakeMailer) string {
	t.Helper()

	messages := mailer.sent()
	if len(messages) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(messages))
	}
	match := regexp.MustCompile(`token=([A-Za-z0-9_-]+)`).FindStringSubmatch(messages[0].Body.Text)
	if match == nil {
		t.Fatalf("no reset token in %q", messages[0].Body.Text)
	}
	return match[1]
}

func TestPasswordResetRequestHTTPAccepted(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	u, _ := createTestUser(t, db, tag)
	client := testClient(t)

	accepted := postJSON(t, client, ts.URL+"/api/v1/users/password-resets", map[string]string{"email": u.Email})
	if accepted.StatusCode != http.StatusAccepted {
		t.Fatalf("status = %d, want 202", accepted.StatusCode)
	}
	accepted.Body.Close()

	unknown := postJSON(t, client, ts.URL+"/api/v1/users/password-resets", map[string]string{"email": "nobody-" + tag + "@example.com"})
	if unknown.StatusCode != http.StatusAccepted {
		t.Fatalf("unknown email status = %d, want 202", unknown.StatusCode)
	}
	unknown.Body.Close()

	if got := len(mailer.sent()); got != 1 {
		t.Fatalf("sent messages = %d, want 1", got)
	}
}

func TestPasswordResetRequestDisabledHTTP(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	setResetSite(t, settingsService)

	u, _ := createTestUser(t, db, tag)
	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets", map[string]string{"email": u.Email})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	code, param := decodeErrorFields(t, resp)
	if code != "password_reset_disabled" || param != "" {
		t.Fatalf("error = %q param %q, want password_reset_disabled with no param", code, param)
	}
	if got := len(mailer.sent()); got != 0 {
		t.Fatalf("sent messages = %d, want 0", got)
	}
}

func TestPasswordResetConfirmDisabledHTTP(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	u, _ := createTestUser(t, db, tag)
	client := testClient(t)
	request := postJSON(t, client, ts.URL+"/api/v1/users/password-resets", map[string]string{"email": u.Email})
	request.Body.Close()
	if request.StatusCode != http.StatusAccepted {
		t.Fatalf("request status = %d, want 202", request.StatusCode)
	}
	token := resetTokenFrom(t, mailer)

	if err := settingsService.SetBool(ctx, "auth", "password_reset_enabled", false); err != nil {
		t.Fatalf("disable reset: %v", err)
	}

	resp := postJSON(t, client, ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": token, "new_password": "BrandNew123",
	})
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
	code, _ := decodeErrorFields(t, resp)
	if code != "password_reset_disabled" {
		t.Fatalf("code = %q, want password_reset_disabled", code)
	}

	count, err := db.Client.PasswordReset.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	if count != 1 {
		t.Fatalf("reset rows while disabled = %d, want 1 (the token must survive, not be deleted)", count)
	}
}

func TestPasswordResetRequestInvalidEmailHTTP(t *testing.T) {
	db := testDB(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets", map[string]string{"email": "not-an-email"})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	code, param := decodeErrorFields(t, resp)
	if code != "invalid_request" || param != "email" {
		t.Fatalf("error = %q param %q, want invalid_request param email", code, param)
	}
}

func TestPasswordResetRequestMailUnavailableHTTP(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: false}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	u, _ := createTestUser(t, db, tag)
	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets", map[string]string{"email": u.Email})
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
	code, _ := decodeErrorFields(t, resp)
	if code != "mail_unavailable" {
		t.Fatalf("code = %q, want mail_unavailable", code)
	}

	unknown := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets", map[string]string{"email": "nobody-" + tag + "@example.com"})
	if unknown.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unknown email status = %d, want 503 before the lookup", unknown.StatusCode)
	}
	unknown.Body.Close()
}

func TestPasswordResetConfirmHTTP(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	u, password := createTestUser(t, db, tag)
	oldClient := loginAs(t, ts.URL, u.Email, password)

	request := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets", map[string]string{"email": u.Email})
	request.Body.Close()
	if request.StatusCode != http.StatusAccepted {
		t.Fatalf("request status = %d, want 202", request.StatusCode)
	}
	token := resetTokenFrom(t, mailer)

	client := testClient(t)
	confirm := postJSON(t, client, ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": token, "new_password": "BrandNew123",
	})
	if confirm.StatusCode != http.StatusOK {
		body := decodeErrorBody(t, confirm)
		t.Fatalf("confirm status = %d, want 200 (body = %v)", body["status"], body)
	}
	if cookies := confirm.Header.Values("Set-Cookie"); len(cookies) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", cookies)
	}

	if status := meStatus(t, oldClient, ts.URL); status != http.StatusUnauthorized {
		t.Fatalf("old session me = %d, want 401", status)
	}

	oldLogin := loginRaw(t, testClient(t), ts.URL, u.Email, password)
	oldLogin.Body.Close()
	if oldLogin.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, want 401", oldLogin.StatusCode)
	}

	newClient := loginAs(t, ts.URL, u.Email, "BrandNew123")
	if status := meStatus(t, newClient, ts.URL); status != http.StatusOK {
		t.Fatalf("new session me = %d, want 200", status)
	}
}

func decodeErrorBody(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return map[string]any{"undecoded": true}
	}
	return body
}

func TestPasswordResetConfirmInvalidTokenHTTP(t *testing.T) {
	db := testDB(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": "never-issued", "new_password": "BrandNew123",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	code, param := decodeErrorFields(t, resp)
	if code != "password_reset_invalid" || param != "token" {
		t.Fatalf("error = %q param %q, want password_reset_invalid param token", code, param)
	}
}

func TestPasswordResetConfirmWeakPasswordHTTP(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	u, _ := createTestUser(t, db, tag)
	request := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets", map[string]string{"email": u.Email})
	request.Body.Close()
	token := resetTokenFrom(t, mailer)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": token, "new_password": "abc",
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	code, param := decodeErrorFields(t, resp)
	if code != "weak_password" || param != "new_password" {
		t.Fatalf("error = %q param %q, want weak_password param new_password", code, param)
	}

	after := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": token, "new_password": "BrandNew123",
	})
	if after.StatusCode != http.StatusOK {
		t.Fatalf("status after the rejected password = %d, want 200 (the token must not be consumed)", after.StatusCode)
	}
	after.Body.Close()
}

func TestPasswordResetConfirmSamePasswordHTTP(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	u, password := createTestUser(t, db, tag)
	request := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets", map[string]string{"email": u.Email})
	request.Body.Close()
	token := resetTokenFrom(t, mailer)

	resp := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": token, "new_password": password,
	})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	code, param := decodeErrorFields(t, resp)
	if code != "password_unchanged" || param != "new_password" {
		t.Fatalf("error = %q param %q, want password_unchanged param new_password", code, param)
	}
}

func TestPasswordResetConfirmMissingFieldsHTTP(t *testing.T) {
	db := testDB(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	emptyToken := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": "", "new_password": "BrandNew123",
	})
	if emptyToken.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", emptyToken.StatusCode)
	}
	code, param := decodeErrorFields(t, emptyToken)
	if code != "invalid_request" || param != "token" {
		t.Fatalf("error = %q param %q, want invalid_request param token", code, param)
	}

	emptyPassword := postJSON(t, testClient(t), ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": "whatever", "new_password": "",
	})
	if emptyPassword.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", emptyPassword.StatusCode)
	}
	code, param = decodeErrorFields(t, emptyPassword)
	if code != "invalid_request" || param != "new_password" {
		t.Fatalf("error = %q param %q, want invalid_request param new_password", code, param)
	}
}

func TestPasswordResetRequestRateLimitHTTP(t *testing.T) {
	db := testDB(t)
	tag := uniqueTag(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	client := testClient(t)
	email := "rate-" + tag + "@example.com"
	for range 3 {
		resp := postJSON(t, client, ts.URL+"/api/v1/users/password-resets", map[string]string{"email": email})
		resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Fatalf("status inside the limit = %d, want 202", resp.StatusCode)
		}
	}

	resp := postJSON(t, client, ts.URL+"/api/v1/users/password-resets", map[string]string{"email": email})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	resp.Body.Close()
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter == "" {
		t.Fatal("Retry-After header missing on 429")
	}
}

func TestPasswordResetConfirmRateLimitHTTP(t *testing.T) {
	db := testDB(t)
	mailer := &fakeMailer{configured: true}
	ts, _, settingsService := newTestAppWithMailer(t, db, mailer)
	enableReset(t, settingsService)
	setResetSite(t, settingsService)

	client := testClient(t)
	for range 30 {
		resp := postJSON(t, client, ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
			"token": "never-issued", "new_password": "BrandNew123",
		})
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status inside the limit = %d, want 400", resp.StatusCode)
		}
	}

	resp := postJSON(t, client, ts.URL+"/api/v1/users/password-resets/confirmations", map[string]string{
		"token": "never-issued", "new_password": "BrandNew123",
	})
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", resp.StatusCode)
	}
	resp.Body.Close()
	if retryAfter := resp.Header.Get("Retry-After"); retryAfter == "" {
		t.Fatal("Retry-After header missing on 429")
	}
}
