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

func TestLoadDatabaseDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable")
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
