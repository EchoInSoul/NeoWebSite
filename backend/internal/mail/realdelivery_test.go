package mail

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"website/backend/internal/config"
)

func TestRealDelivery(t *testing.T) {
	if os.Getenv("SMTP_HOST") == "" {
		_ = config.LoadDotEnvFile("../../.env")
	}

	host := os.Getenv("SMTP_HOST")
	from := os.Getenv("SMTP_FROM")
	recipient := os.Getenv("MAIL_TEST_RECIPIENT")
	if host == "" || from == "" || recipient == "" {
		t.Skip("SKIP: SMTP_HOST / SMTP_FROM / MAIL_TEST_RECIPIENT are not set")
	}

	port := 587
	if value := os.Getenv("SMTP_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			t.Fatalf("SMTP_PORT: %v", err)
		}
		port = parsed
	}

	tlsMode := config.SMTPTLSStartTLS
	if os.Getenv("SMTP_TLS") == string(config.SMTPTLSSSL) {
		tlsMode = config.SMTPTLSSSL
	}

	fromName := os.Getenv("SMTP_FROM_NAME")
	if fromName == "" {
		fromName = "NeoBlog"
	}

	cfg := config.Config{
		SMTPHost:     host,
		SMTPPort:     port,
		SMTPTLS:      tlsMode,
		SMTPUsername: os.Getenv("SMTP_USERNAME"),
		SMTPPassword: os.Getenv("SMTP_PASSWORD"),
		SMTPFrom:     from,
		SMTPFromName: fromName,
	}

	body, err := Render("example", map[string]string{"Time": time.Now().Format("2006-01-02 15:04:05"), "Site": "NeoBlog"})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	err = newSMTPTransport(cfg).deliver(ctx, Message{To: recipient, Subject: "NeoBlog 邮件配置自检", Body: body})
	if err != nil {
		t.Fatalf("deliver: %v", err)
	}
}
