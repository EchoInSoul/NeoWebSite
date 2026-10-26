package mail

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"website/backend/internal/common/guard"
	"website/backend/internal/common/rbac"
)

type stubMailer struct {
	err           error
	got           Message
	calls         int
	configured    bool
	configuredErr error
}

func (s *stubMailer) Send(_ context.Context, _ Message) error { return nil }

func (s *stubMailer) SendSync(_ context.Context, msg Message) error {
	s.calls++
	s.got = msg
	return s.err
}

func (s *stubMailer) Configured(_ context.Context) (bool, error) {
	return s.configured, s.configuredErr
}

func (s *stubMailer) Close(_ context.Context) error { return nil }

type controllerSettingsStub struct {
	groups map[string]map[string]any
	errs   map[string]error
}

func (s *controllerSettingsStub) GetGroup(_ context.Context, group string) (map[string]any, error) {
	if err, ok := s.errs[group]; ok {
		return nil, err
	}
	values, ok := s.groups[group]
	if !ok {
		return nil, errors.New("settings group not found")
	}
	return values, nil
}

func newControllerTestServer(t *testing.T, mailer Mailer, groupKey string, settings ...Settings) *httptest.Server {
	t.Helper()

	var settingsService Settings
	if len(settings) > 0 {
		settingsService = settings[0]
	}

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(guard.SessionIdentityKey, guard.Identity{UserID: "u1", GroupKey: groupKey, Permissions: rbac.PresetPermissions(groupKey)})
		c.Next()
	})
	newController(mailer, settingsService).registerProtected(engine.Group(""))

	return httptest.NewServer(engine)
}

func postMessage(t *testing.T, ts *httptest.Server, body string) *http.Response {
	t.Helper()

	resp, err := http.Post(ts.URL+"/admin/mail/messages", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func decodeErrorEnvelope(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("error field = %v, want nested object", body["error"])
	}
	return errObj
}

func TestCreateMessageDeliversSelfCheck(t *testing.T) {
	mailer := &stubMailer{}
	ts := newControllerTestServer(t, mailer, "owner")

	resp := postMessage(t, ts, `{"to":"user@example.com"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}

	var body createMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.To != "user@example.com" || body.Subject != selfCheckSubject || !body.Sent {
		t.Fatalf("response = %+v, want to/subject/sent", body)
	}
	if mailer.calls != 1 {
		t.Fatalf("send sync calls = %d, want 1", mailer.calls)
	}
	if mailer.got.To != "user@example.com" {
		t.Fatalf("sent to = %q, want user@example.com", mailer.got.To)
	}
	if mailer.got.Subject != selfCheckSubject {
		t.Fatalf("subject = %q, want %q", mailer.got.Subject, selfCheckSubject)
	}
	if !strings.Contains(mailer.got.Body.Text, "邮件配置自检") || !strings.Contains(mailer.got.Body.HTML, "邮件配置自检") {
		t.Fatalf("body not rendered: text=%q html=%q", mailer.got.Body.Text, mailer.got.Body.HTML)
	}
}

func TestCreateMessageExtractsAddressFromDisplayName(t *testing.T) {
	mailer := &stubMailer{}
	ts := newControllerTestServer(t, mailer, "admin")

	resp := postMessage(t, ts, `{"to":"NeoBlog Tester <me@example.com>"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if mailer.got.To != "me@example.com" {
		t.Fatalf("sent to = %q, want me@example.com", mailer.got.To)
	}
}

func TestCreateMessageRendersSelectedTemplate(t *testing.T) {
	mailer := &stubMailer{}
	settings := &controllerSettingsStub{groups: map[string]map[string]any{
		"site": {
			"title": "My Blog",
			"url":   "https://blog.example.com",
		},
		"mail": {
			"signup_code_subject": "自定义注册验证码",
			"signup_code_body":    "<p>{{.siteTitle}} / {{.username}} / {{.code}} / {{.expiresMinutes}} / {{.siteUrl}}</p>",
		},
	}}
	ts := newControllerTestServer(t, mailer, "owner", settings)

	resp := postMessage(t, ts, `{"to":"me@example.com","template":"signup_code"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if mailer.calls != 1 {
		t.Fatalf("send sync calls = %d, want 1", mailer.calls)
	}
	if mailer.got.Subject != "自定义注册验证码" {
		t.Fatalf("subject = %q, want custom subject", mailer.got.Subject)
	}
	for _, want := range []string{"My Blog", "me", "123456", "30", "https://blog.example.com"} {
		if !strings.Contains(mailer.got.Body.HTML, want) {
			t.Fatalf("html body %q missing %q", mailer.got.Body.HTML, want)
		}
	}
	if !strings.Contains(mailer.got.Body.Text, "https://example.com/register/verify?token=preview-token") {
		t.Fatalf("text body = %q, want built-in signup preview", mailer.got.Body.Text)
	}
}

func TestCreateMessageUsesDefaultSignupSubject(t *testing.T) {
	mailer := &stubMailer{}
	ts := newControllerTestServer(t, mailer, "owner")

	resp := postMessage(t, ts, `{"to":"me@example.com","template":"signup_code"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if mailer.got.Subject != "注册验证邮件" {
		t.Fatalf("subject = %q, want default signup subject", mailer.got.Subject)
	}
	if !strings.Contains(mailer.got.Body.Text, "https://example.com/register/verify?token=preview-token") {
		t.Fatalf("text body = %q, want verify link", mailer.got.Body.Text)
	}
}

func TestCreateMessageUsesBuiltinTemplateDefaults(t *testing.T) {
	mailer := &stubMailer{}
	ts := newControllerTestServer(t, mailer, "owner")

	resp := postMessage(t, ts, `{"to":"me@example.com","template":"password_reset"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if mailer.got.Subject != "密码重置" {
		t.Fatalf("subject = %q, want default password reset subject", mailer.got.Subject)
	}
	if !strings.Contains(mailer.got.Body.HTML, "https://example.com/reset-password/confirm?token=preview-token") {
		t.Fatalf("html = %q, want reset link", mailer.got.Body.HTML)
	}
	if !strings.Contains(mailer.got.Body.Text, "https://example.com/reset-password/confirm?token=preview-token") {
		t.Fatalf("text = %q, want reset link", mailer.got.Body.Text)
	}
	if strings.Contains(mailer.got.Body.Text, "123456") {
		t.Fatalf("text = %q, still carries the preview code", mailer.got.Body.Text)
	}
}

func TestCreateMessageUsesDefaultRecoveryCodesSubject(t *testing.T) {
	mailer := &stubMailer{}
	ts := newControllerTestServer(t, mailer, "owner")

	resp := postMessage(t, ts, `{"to":"me@example.com","template":"recovery_codes"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if mailer.got.Subject != "两步验证恢复码" {
		t.Fatalf("subject = %q, want default recovery codes subject", mailer.got.Subject)
	}
	if !strings.Contains(mailer.got.Body.Text, "AAAA-BBBB-CCCC-DDDD") {
		t.Fatalf("text = %q, want preview codes", mailer.got.Body.Text)
	}
	if !strings.Contains(mailer.got.Body.HTML, "AAAA-BBBB-CCCC-DDDD") {
		t.Fatalf("html = %q, want preview codes", mailer.got.Body.HTML)
	}
}

func TestComposePasswordResetRendersLink(t *testing.T) {
	body, err := Render(TemplatePasswordReset, map[string]string{
		"siteTitle": "My Blog", "siteUrl": "https://blog.example.com",
		"username": "alice", "expiresMinutes": "30", "sentAt": "2026-10-26 12:00:00",
		"resetUrl": "https://blog.example.com/reset-password/confirm?token=abc", "code": "",
	})
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(body.Text, "https://blog.example.com/reset-password/confirm?token=abc") {
		t.Fatalf("text = %q, want reset link", body.Text)
	}
	if !strings.Contains(body.HTML, "https://blog.example.com/reset-password/confirm?token=abc") {
		t.Fatalf("html = %q, want reset link", body.HTML)
	}
	if strings.Contains(body.Text, "验证码") || strings.Contains(body.HTML, "验证码") {
		t.Fatalf("body still mentions a code: text=%q html=%q", body.Text, body.HTML)
	}
}

func TestPasswordResetCustomBodyKeepsEmptyCode(t *testing.T) {
	settings := &controllerSettingsStub{groups: map[string]map[string]any{
		"mail": {"password_reset_body": "<p>{{.code}}</p>"},
	}}

	_, body, err := Compose(context.Background(), settings, TemplatePasswordReset, map[string]string{"code": ""})
	if err != nil {
		t.Fatalf("Compose: %v", err)
	}
	if strings.Contains(body.HTML, "<no value>") {
		t.Fatalf("html = %q, custom body broke on the empty code placeholder", body.HTML)
	}
}

func TestCreateMessageRejectsUnknownTemplate(t *testing.T) {
	mailer := &stubMailer{}
	ts := newControllerTestServer(t, mailer, "owner")

	resp := postMessage(t, ts, `{"to":"me@example.com","template":"bogus"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
	errObj := decodeErrorEnvelope(t, resp)
	if errObj["code"] != "invalid_request" || errObj["param"] != "template" {
		t.Fatalf("error = %v, want invalid_request param template", errObj)
	}
	if mailer.calls != 0 {
		t.Fatalf("send sync calls = %d, want 0", mailer.calls)
	}
}

func TestCreateMessageValidatesRecipient(t *testing.T) {
	for _, who := range []struct {
		name       string
		body       string
		wantParams bool
	}{
		{name: "not an email", body: `{"to":"not-an-email"}`, wantParams: true},
		{name: "missing to", body: `{}`, wantParams: true},
		{name: "malformed json", body: `{"to"`, wantParams: false},
	} {
		mailer := &stubMailer{}
		ts := newControllerTestServer(t, mailer, "owner")

		resp := postMessage(t, ts, who.body)
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", who.name, resp.StatusCode)
		}
		errObj := decodeErrorEnvelope(t, resp)
		if errObj["code"] != "invalid_request" {
			t.Fatalf("%s: code = %v, want invalid_request", who.name, errObj["code"])
		}
		if who.wantParams && errObj["param"] != "to" {
			t.Fatalf("%s: param = %v, want to", who.name, errObj["param"])
		}
		if !who.wantParams && errObj["param"] != nil {
			t.Fatalf("%s: param = %v, want nil", who.name, errObj["param"])
		}
		if mailer.calls != 0 {
			t.Fatalf("%s: send sync calls = %d, want 0", who.name, mailer.calls)
		}
	}
}

func TestCreateMessageRestrictedToPresetAdminGroups(t *testing.T) {
	for _, groupKey := range []string{"user", ""} {
		mailer := &stubMailer{}
		ts := newControllerTestServer(t, mailer, groupKey)

		resp := postMessage(t, ts, `{"to":"user@example.com"}`)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("group %q: status = %d, want 403", groupKey, resp.StatusCode)
		}
		errObj := decodeErrorEnvelope(t, resp)
		if errObj["code"] != "forbidden" {
			t.Fatalf("group %q: code = %v, want forbidden", groupKey, errObj["code"])
		}
		if mailer.calls != 0 {
			t.Fatalf("group %q: send sync calls = %d, want 0", groupKey, mailer.calls)
		}
	}
}

func TestCreateMessageMapsDeliveryFailure(t *testing.T) {
	mailer := &stubMailer{err: errors.New("dial tcp 127.0.0.1:587: connect refused")}
	ts := newControllerTestServer(t, mailer, "owner")

	resp := postMessage(t, ts, `{"to":"user@example.com"}`)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	errObj := decodeErrorEnvelope(t, resp)
	if errObj["code"] != "mail_delivery_failed" {
		t.Fatalf("code = %v, want mail_delivery_failed", errObj["code"])
	}
	if errObj["type"] != "server_error" {
		t.Fatalf("type = %v, want server_error", errObj["type"])
	}
	if mailer.calls != 1 {
		t.Fatalf("send sync calls = %d, want 1", mailer.calls)
	}
}

func TestMailStatus(t *testing.T) {
	settings := &controllerSettingsStub{groups: map[string]map[string]any{
		"site": {"url": "https://example.com"},
	}}
	mailer := &stubMailer{configured: true}
	ts := newControllerTestServer(t, mailer, rbac.GroupKeyOwner, settings)

	resp, err := http.Get(ts.URL + "/admin/mail/status")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		SMTPConfigured    bool `json:"smtp_configured"`
		SiteURLConfigured bool `json:"site_url_configured"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !body.SMTPConfigured || !body.SiteURLConfigured {
		t.Fatalf("body = %+v, want both true", body)
	}
}

func TestMailStatusSMTPFalseAndReadError(t *testing.T) {
	siteSettings := &controllerSettingsStub{groups: map[string]map[string]any{
		"site": {"url": "https://example.com"},
	}}

	t.Run("smtp false", func(t *testing.T) {
		ts := newControllerTestServer(t, &stubMailer{configured: false}, rbac.GroupKeyOwner, siteSettings)
		resp, err := http.Get(ts.URL + "/admin/mail/status")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		var body struct {
			SMTPConfigured    bool `json:"smtp_configured"`
			SiteURLConfigured bool `json:"site_url_configured"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if body.SMTPConfigured || !body.SiteURLConfigured {
			t.Fatalf("body = %+v, want smtp false and site true", body)
		}
	})

	t.Run("configured read error", func(t *testing.T) {
		ts := newControllerTestServer(t, &stubMailer{configuredErr: errors.New("db down")}, rbac.GroupKeyOwner, siteSettings)
		resp, err := http.Get(ts.URL + "/admin/mail/status")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", resp.StatusCode)
		}
	})

	t.Run("site read error", func(t *testing.T) {
		settings := &controllerSettingsStub{errs: map[string]error{"site": errors.New("db down")}}
		ts := newControllerTestServer(t, &stubMailer{configured: true}, rbac.GroupKeyOwner, settings)
		resp, err := http.Get(ts.URL + "/admin/mail/status")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500", resp.StatusCode)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		ts := newControllerTestServer(t, &stubMailer{configured: true}, rbac.GroupKeyUser, siteSettings)
		resp, err := http.Get(ts.URL + "/admin/mail/status")
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})
}

func TestComposeSettingsReadError(t *testing.T) {
	settings := &controllerSettingsStub{errs: map[string]error{"mail": errors.New("db down")}}
	if _, _, err := Compose(context.Background(), settings, TemplateSignupCode, map[string]string{}); err == nil {
		t.Fatal("Compose returned nil error when mail settings read failed")
	}
}

func TestComposeRejectsBadCustomBody(t *testing.T) {
	settings := &controllerSettingsStub{groups: map[string]map[string]any{
		"mail": {"signup_code_body": "{{"},
	}}
	if _, _, err := Compose(context.Background(), settings, TemplateSignupCode, map[string]string{"verifyUrl": "https://example.com/x"}); err == nil {
		t.Fatal("Compose accepted bad custom body")
	}
}
