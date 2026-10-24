package mail

import (
	"context"
	"errors"
	"strings"
	"testing"

	"website/backend/internal/config"
)

type settingsStub struct {
	values map[string]any
	err    error
	calls  int
}

func (s *settingsStub) GetGroup(_ context.Context, _ string) (map[string]any, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}
	return s.values, nil
}

func mailSettingsValues() map[string]any {
	return map[string]any{
		"enabled":                true,
		"smtp_host":              "smtp.example.com",
		"smtp_port":              int64(465),
		"smtp_encryption":        "ssl",
		"smtp_username":          "sender@example.com",
		"smtp_password":          "secret",
		"sender_address":         "sender@example.com",
		"sender_name":            "Example",
		"signup_code_subject":    "",
		"signup_code_body":       "",
		"password_reset_subject": "",
		"password_reset_body":    "",
	}
}

func TestResolveTransportConfigWithoutSettingsUsesEnvironment(t *testing.T) {
	env := config.Config{SMTPHost: "env.example.com", SMTPPort: 587, SMTPFrom: "env@example.com"}

	cfg, send, err := resolveTransportConfig(context.Background(), env, nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !send || cfg.SMTPHost != "env.example.com" {
		t.Fatalf("cfg = %+v send = %v, want environment config", cfg, send)
	}

	cfg, send, err = resolveTransportConfig(context.Background(), config.Config{}, nil)
	if err != nil {
		t.Fatalf("resolve empty: %v", err)
	}
	if send || cfg.SMTPHost != "" {
		t.Fatalf("cfg = %+v send = %v, want log mode", cfg, send)
	}
}

func TestResolveTransportConfigUsesStoredSettings(t *testing.T) {
	stub := &settingsStub{values: mailSettingsValues()}

	cfg, send, err := resolveTransportConfig(context.Background(), config.Config{}, stub)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !send {
		t.Fatal("stored SMTP settings must enable sending")
	}
	if cfg.SMTPHost != "smtp.example.com" || cfg.SMTPPort != 465 {
		t.Fatalf("host/port = %q/%d, want smtp.example.com/465", cfg.SMTPHost, cfg.SMTPPort)
	}
	if cfg.SMTPTLS != config.SMTPTLSSSL {
		t.Fatalf("tls = %q, want ssl", cfg.SMTPTLS)
	}
	if cfg.SMTPFrom != "sender@example.com" || cfg.SMTPFromName != "Example" {
		t.Fatalf("sender = %q/%q, want sender@example.com/Example", cfg.SMTPFrom, cfg.SMTPFromName)
	}
	if cfg.SMTPUsername != "sender@example.com" || cfg.SMTPPassword != "secret" {
		t.Fatalf("auth = %q/%q, want stored credentials", cfg.SMTPUsername, cfg.SMTPPassword)
	}
	if stub.calls != 1 {
		t.Fatalf("settings reads = %d, want 1", stub.calls)
	}
}

func TestResolveTransportConfigFallsBackToEnvironmentWhenHostEmpty(t *testing.T) {
	values := mailSettingsValues()
	values["smtp_host"] = ""
	stub := &settingsStub{values: values}
	env := config.Config{SMTPHost: "env.example.com", SMTPPort: 587, SMTPFrom: "env@example.com"}

	cfg, send, err := resolveTransportConfig(context.Background(), env, stub)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !send || cfg.SMTPHost != "env.example.com" || cfg.SMTPFrom != "env@example.com" {
		t.Fatalf("cfg = %+v send = %v, want environment fallback", cfg, send)
	}
}

func TestResolveTransportConfigLogModeKeepsStoredSender(t *testing.T) {
	values := mailSettingsValues()
	values["smtp_host"] = ""
	stub := &settingsStub{values: values}

	cfg, send, err := resolveTransportConfig(context.Background(), config.Config{}, stub)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if send {
		t.Fatal("empty host without environment SMTP must use log mode")
	}
	if cfg.SMTPFrom != "sender@example.com" || cfg.SMTPFromName != "Example" {
		t.Fatalf("log sender = %q/%q, want stored sender", cfg.SMTPFrom, cfg.SMTPFromName)
	}
}

func TestResolveTransportConfigDisabledUsesLogMode(t *testing.T) {
	values := mailSettingsValues()
	values["enabled"] = false
	stub := &settingsStub{values: values}
	env := config.Config{SMTPHost: "env.example.com", SMTPPort: 587, SMTPFrom: "env@example.com"}

	cfg, send, err := resolveTransportConfig(context.Background(), env, stub)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if send {
		t.Fatal("disabled mail settings must not send")
	}
	if cfg.SMTPFrom != "sender@example.com" || cfg.SMTPFromName != "Example" {
		t.Fatalf("log sender = %q/%q, want stored sender", cfg.SMTPFrom, cfg.SMTPFromName)
	}
}

func TestResolveTransportConfigRejectsInvalidSettings(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{"read failure", nil, "mail: read settings"},
		{"unknown encryption", func(values map[string]any) { values["smtp_encryption"] = "none" }, "smtp_encryption"},
		{"port out of range", func(values map[string]any) { values["smtp_port"] = int64(70000) }, "smtp_port"},
		{"missing sender", func(values map[string]any) { values["sender_address"] = "" }, "sender_address"},
		{"invalid sender", func(values map[string]any) { values["sender_address"] = "noreply" }, "sender_address"},
		{"username without password", func(values map[string]any) { values["smtp_password"] = "" }, "smtp_password"},
	} {
		values := mailSettingsValues()
		if tc.mutate != nil {
			tc.mutate(values)
		}
		stub := &settingsStub{values: values}
		if tc.name == "read failure" {
			stub.err = errors.New("database down")
		}

		_, _, err := resolveTransportConfig(context.Background(), config.Config{}, stub)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: err = %v, want substring %q", tc.name, err, tc.want)
		}
	}
}

func TestSettingsTransportPropagatesSettingsErrors(t *testing.T) {
	stub := &settingsStub{err: errors.New("database down")}
	tr := newSettingsTransport(config.Config{}, stub, discardLogger())

	err := tr.deliver(context.Background(), testMessage())
	if err == nil || !strings.Contains(err.Error(), "read settings") {
		t.Fatalf("deliver = %v, want settings read error", err)
	}
}

func TestSettingsTransportConfiguredReadError(t *testing.T) {
	tr := newSettingsTransport(config.Config{}, &settingsStub{err: errors.New("database down")}, discardLogger())
	ok, err := tr.configured(context.Background())
	if err == nil || ok {
		t.Fatalf("configured = (%v, %v), want error", ok, err)
	}
}

func TestSettingsTransportConfiguredEnvFallback(t *testing.T) {
	env := config.Config{SMTPHost: "env.example.com", SMTPPort: 587, SMTPFrom: ""}
	values := mailSettingsValues()
	values["smtp_host"] = ""
	tr := newSettingsTransport(env, &settingsStub{values: values}, discardLogger())

	ok, err := tr.configured(context.Background())
	if err == nil && ok {
		t.Fatal("configured = true with missing SMTP_FROM, want false or error")
	}
}

func TestSettingsTransportRejectsRequireSMTPWhenLog(t *testing.T) {
	logger, buffer := testLogger(t)
	values := mailSettingsValues()
	values["enabled"] = false
	tr := newSettingsTransport(config.Config{}, &settingsStub{values: values}, logger)

	msg := testMessage()
	msg.Body = Body{Text: "secret-token", HTML: "<p>secret-token</p>"}
	msg.RequireSMTP = true
	if err := tr.deliver(context.Background(), msg); err == nil {
		t.Fatal("deliver with RequireSMTP to log sender = nil, want error")
	}
	if strings.Contains(buffer.String(), "secret-token") {
		t.Fatalf("log contains message body: %s", buffer.String())
	}
}
