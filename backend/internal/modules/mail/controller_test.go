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
)

type stubMailer struct {
	err   error
	got   Message
	calls int
}

func (s *stubMailer) Send(_ context.Context, _ Message) error { return nil }

func (s *stubMailer) SendSync(_ context.Context, msg Message) error {
	s.calls++
	s.got = msg
	return s.err
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
		c.Set(guard.SessionIdentityKey, guard.Identity{UserID: "u1", GroupKey: groupKey})
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
	for _, want := range []string{"My Blog", "me", "123456", "10", "https://blog.example.com"} {
		if !strings.Contains(mailer.got.Body.HTML, want) {
			t.Fatalf("html body %q missing %q", mailer.got.Body.HTML, want)
		}
	}
	if !strings.Contains(mailer.got.Body.Text, "注册验证码") || !strings.Contains(mailer.got.Body.Text, "123456") {
		t.Fatalf("text body = %q, want built-in signup preview", mailer.got.Body.Text)
	}
}

func TestCreateMessageUsesBuiltinTemplateDefaults(t *testing.T) {
	mailer := &stubMailer{}
	ts := newControllerTestServer(t, mailer, "owner")

	resp := postMessage(t, ts, `{"to":"me@example.com","template":"password_reset"}`)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	if mailer.got.Subject != "NeoBlog 密码重置验证码" {
		t.Fatalf("subject = %q, want default password reset subject", mailer.got.Subject)
	}
	if !strings.Contains(mailer.got.Body.HTML, "密码重置验证码") || !strings.Contains(mailer.got.Body.Text, "123456") {
		t.Fatalf("body not rendered: html=%q text=%q", mailer.got.Body.HTML, mailer.got.Body.Text)
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
