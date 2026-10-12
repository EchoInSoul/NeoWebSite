package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected error naming DATABASE_URL, got %v", err)
	}
}

func TestLoadFallsBackToCompiledDefaultEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	unsetEnv(t, "APP_ENV")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment != DefaultEnvironment {
		t.Fatalf("Environment = %q, want compiled default %q", cfg.Environment, DefaultEnvironment)
	}
	if cfg.IsProduction() != (DefaultEnvironment == EnvironmentProduction) {
		t.Fatalf("IsProduction = %v, want %v", cfg.IsProduction(), DefaultEnvironment == EnvironmentProduction)
	}
}

func TestCompiledDefaultEnvironmentIsKnown(t *testing.T) {
	switch DefaultEnvironment {
	case EnvironmentDevelopment, EnvironmentProduction, EnvironmentTest:
	default:
		t.Fatalf("DefaultEnvironment = %q, which no build mode accepts", DefaultEnvironment)
	}
}

func TestLoadRejectsEmptyEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)

	for _, value := range []string{"", " ", "production x"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APP_ENV", value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
				t.Fatalf("expected error mentioning APP_ENV, got %v", err)
			}
		})
	}
}

func TestLoadEnvironmentOverridesCompiledDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)

	for _, value := range []string{EnvironmentDevelopment, EnvironmentProduction, EnvironmentTest} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APP_ENV", value)
			cfg, err := Load()
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Environment != value {
				t.Fatalf("Environment = %q, want %q", cfg.Environment, value)
			}
			if cfg.IsProduction() != (value == EnvironmentProduction) {
				t.Fatalf("IsProduction = %v for %q", cfg.IsProduction(), value)
			}
		})
	}
}

func TestLoadRejectsUnknownEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)

	for _, value := range []string{"prod", "Production", "staging", "dev"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("APP_ENV", value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "APP_ENV") {
				t.Fatalf("expected error mentioning APP_ENV, got %v", err)
			}
		})
	}
}

func TestLoadDatabaseDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL == "" || cfg.DBMaxOpenConns != 10 || cfg.DBMaxIdleConns != 5 || cfg.DBConnMaxLifetime != 30*time.Minute {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestLoadReadsPoolOverrides(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	t.Setenv("DB_MAX_OPEN_CONNS", "20")
	t.Setenv("DB_MAX_IDLE_CONNS", "8")
	t.Setenv("DB_CONN_MAX_LIFETIME", "1h")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DBMaxOpenConns != 20 || cfg.DBMaxIdleConns != 8 || cfg.DBConnMaxLifetime != time.Hour {
		t.Fatalf("unexpected overrides: %+v", cfg)
	}
}

func TestLoadRejectsInvalidPoolValues(t *testing.T) {
	cases := []struct {
		key, value, wantMention string
	}{
		{"DB_MAX_OPEN_CONNS", "0", "DB_MAX_OPEN_CONNS"},
		{"DB_MAX_OPEN_CONNS", "abc", "DB_MAX_OPEN_CONNS"},
		{"DB_MAX_IDLE_CONNS", "-1", "DB_MAX_IDLE_CONNS"},
		{"DB_CONN_MAX_LIFETIME", "soon", "DB_CONN_MAX_LIFETIME"},
	}
	for _, tc := range cases {
		t.Run(tc.key+"/"+tc.value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
			t.Setenv(tc.key, tc.value)
			if _, err := Load(); err == nil || !strings.Contains(err.Error(), tc.wantMention) {
				t.Fatalf("expected error mentioning %s, got %v", tc.wantMention, err)
			}
		})
	}
}

func TestLoadRejectsIdleGreaterThanOpen(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("DB_MAX_OPEN_CONNS", "5")
	t.Setenv("DB_MAX_IDLE_CONNS", "9")
	if _, err := Load(); err == nil {
		t.Fatal("expected error when DB_MAX_IDLE_CONNS > DB_MAX_OPEN_CONNS")
	}
}

func TestLoadSessionLifetimeDefaultsToSevenDays(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionLifetime != 168*time.Hour {
		t.Fatalf("SessionLifetime = %v, want 168h", cfg.SessionLifetime)
	}
}

func TestLoadReadsSessionLifetimeOverride(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	t.Setenv("SESSION_LIFETIME", "1h")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionLifetime != time.Hour {
		t.Fatalf("SessionLifetime = %v, want 1h", cfg.SessionLifetime)
	}
}

func TestLoadRejectsInvalidSessionLifetime(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("SESSION_LIFETIME", "soon")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SESSION_LIFETIME") {
		t.Fatalf("expected error mentioning SESSION_LIFETIME, got %v", err)
	}
}

func clearSMTPEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"SMTP_HOST", "SMTP_PORT", "SMTP_TLS", "SMTP_USERNAME", "SMTP_PASSWORD", "SMTP_FROM", "SMTP_FROM_NAME"} {
		t.Setenv(key, "")
	}
}

func TestLoadSMTPDefaultsWhenHostUnset(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	clearSMTPEnv(t)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTPHost != "" || cfg.SMTPPort != 587 || cfg.SMTPTLS != SMTPTLSStartTLS || cfg.SMTPUsername != "" || cfg.SMTPPassword != "" || cfg.SMTPFrom != "" {
		t.Fatalf("unexpected SMTP defaults: %+v", cfg)
	}
	if cfg.SMTPFromName != "NeoBlog" {
		t.Fatalf("SMTPFromName = %q, want NeoBlog", cfg.SMTPFromName)
	}
}

func TestLoadSkipsSMTPValidationWhenHostUnset(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	clearSMTPEnv(t)
	t.Setenv("SMTP_PORT", "abc")
	t.Setenv("SMTP_TLS", "bogus")

	if _, err := Load(); err != nil {
		t.Fatalf("expected no error when SMTP_HOST is unset, got %v", err)
	}
}

func TestLoadRequiresSMTPFromWhenHostSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	clearSMTPEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP_FROM") {
		t.Fatalf("expected error mentioning SMTP_FROM, got %v", err)
	}
}

func TestLoadRequiresSMTPPasswordWhenUsernameSet(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	clearSMTPEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "noreply@example.com")
	t.Setenv("SMTP_USERNAME", "noreply@example.com")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP_PASSWORD") {
		t.Fatalf("expected error mentioning SMTP_PASSWORD, got %v", err)
	}
}

func TestLoadRejectsInvalidSMTPPort(t *testing.T) {
	for _, value := range []string{"abc", "0", "70000"} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
			clearSMTPEnv(t)
			t.Setenv("SMTP_HOST", "smtp.example.com")
			t.Setenv("SMTP_FROM", "noreply@example.com")
			t.Setenv("SMTP_PORT", value)

			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP_PORT") {
				t.Fatalf("expected error mentioning SMTP_PORT, got %v", err)
			}
		})
	}
}

func TestLoadRejectsInvalidSMTPTLS(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	clearSMTPEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_FROM", "noreply@example.com")
	t.Setenv("SMTP_TLS", "bogus")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "SMTP_TLS") {
		t.Fatalf("expected error mentioning SMTP_TLS, got %v", err)
	}
}

func TestLoadReadsSMTPConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)
	clearSMTPEnv(t)
	t.Setenv("SMTP_HOST", "smtp.example.com")
	t.Setenv("SMTP_PORT", "465")
	t.Setenv("SMTP_TLS", "ssl")
	t.Setenv("SMTP_USERNAME", "noreply@example.com")
	t.Setenv("SMTP_PASSWORD", "secret")
	t.Setenv("SMTP_FROM", "noreply@example.com")
	t.Setenv("SMTP_FROM_NAME", "NeoBlog Mail")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SMTPHost != "smtp.example.com" || cfg.SMTPPort != 465 || cfg.SMTPTLS != SMTPTLSSSL || cfg.SMTPUsername != "noreply@example.com" || cfg.SMTPPassword != "secret" || cfg.SMTPFrom != "noreply@example.com" || cfg.SMTPFromName != "NeoBlog Mail" {
		t.Fatalf("unexpected SMTP config: %+v", cfg)
	}
}

const validTOTPEncryptionKey = "AAECAwQFBgcICQoLDA0ODxAREhMUFRYXGBkaGxwdHh8="

func TestLoadRequiresTOTPKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", "")

	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TOTP_ENCRYPTION_KEY") {
		t.Fatalf("expected error mentioning TOTP_ENCRYPTION_KEY, got %v", err)
	}
}

func TestLoadRejectsBadTOTPKey(t *testing.T) {
	for _, value := range []string{"not-base64!!", "c2hvcnQ="} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
			t.Setenv("TOTP_ENCRYPTION_KEY", value)

			if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TOTP_ENCRYPTION_KEY") {
				t.Fatalf("expected error mentioning TOTP_ENCRYPTION_KEY, got %v", err)
			}
		})
	}
}

func TestLoadAcceptsValidTOTPKey(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://u:p@127.0.0.1:5434/db")
	t.Setenv("TOTP_ENCRYPTION_KEY", validTOTPEncryptionKey)

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.TOTPEncryptionKey) != 32 {
		t.Fatalf("TOTPEncryptionKey length = %d, want 32", len(cfg.TOTPEncryptionKey))
	}
}
