package mail

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"website/backend/internal/config"
)

const (
	envFileName    = ".env"
	envSearchDepth = 5
)

func findEnvFile(start string, maxDepth int) string {
	dir := start
	for depth := 0; depth < maxDepth; depth++ {
		candidate := filepath.Join(dir, envFileName)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func loadRealDeliveryEnv() {
	wd, err := os.Getwd()
	if err != nil {
		return
	}
	if path := findEnvFile(wd, envSearchDepth); path != "" {
		_ = config.LoadDotEnvFile(path)
	}
}

func TestFindEnvFile(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "internal", "modules", "mail")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if got := findEnvFile(deep, 3); got != "" {
		t.Fatalf("findEnvFile without .env = %q, want empty", got)
	}

	envPath := filepath.Join(root, envFileName)
	if err := os.WriteFile(envPath, []byte("NEOBLOG_TEST=1\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if got := findEnvFile(deep, 4); got != envPath {
		t.Fatalf("findEnvFile = %q, want %q", got, envPath)
	}
}

func TestFindEnvFileLocatesBackendEnv(t *testing.T) {
	expected := filepath.Join("..", "..", "..", envFileName)
	if _, err := os.Stat(expected); err != nil {
		t.Skip("SKIP: backend/.env is not present")
	}

	absolute, err := filepath.Abs(expected)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if got := findEnvFile(wd, envSearchDepth); got != absolute {
		t.Fatalf("findEnvFile(package dir) = %q, want %q (did the package move without updating the search depth?)", got, absolute)
	}
}

func TestLoadRealDeliveryEnvFindsParentEnv(t *testing.T) {
	root := t.TempDir()
	deep := filepath.Join(root, "internal", "modules", "mail")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, envFileName), []byte("NEOBLOG_MAIL_TEST_MARKER=loaded\n"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	const marker = "NEOBLOG_MAIL_TEST_MARKER"
	original, hadOriginal := os.LookupEnv(marker)
	if err := os.Unsetenv(marker); err != nil {
		t.Fatalf("unset: %v", err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			_ = os.Setenv(marker, original)
		} else {
			_ = os.Unsetenv(marker)
		}
	})
	t.Chdir(deep)

	loadRealDeliveryEnv()

	if got := os.Getenv(marker); got != "loaded" {
		t.Fatalf("marker = %q, want loaded", got)
	}
}

func TestRealDelivery(t *testing.T) {
	if os.Getenv("SMTP_HOST") == "" {
		loadRealDeliveryEnv()
	}

	host := os.Getenv("SMTP_HOST")
	from := os.Getenv("SMTP_FROM")
	recipient := os.Getenv("MAIL_TEST_RECIPIENT")
	if host == "" || from == "" || recipient == "" {
		t.Skip("SKIP: SMTP_HOST / SMTP_FROM / MAIL_TEST_RECIPIENT are not set (environment or backend/.env)")
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
