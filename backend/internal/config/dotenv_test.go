package config

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	content := "# comment\r\n" +
		"\r\n" +
		"DATABASE_URL=postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable\r\n" +
		"export EXPORTED=value\r\n" +
		"QUOTED=\"quoted value\"\r\n" +
		"SINGLE='single value'\r\n" +
		"SPACED =  padded  \r\n"

	values, err := parseDotEnv(content)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]string{
		"DATABASE_URL": "postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable",
		"EXPORTED":     "value",
		"QUOTED":       "quoted value",
		"SINGLE":       "single value",
		"SPACED":       "padded",
	}
	for key, expected := range want {
		if values[key] != expected {
			t.Fatalf("%s: got %q, want %q", key, values[key], expected)
		}
	}
	if len(values) != len(want) {
		t.Fatalf("unexpected keys: %v", values)
	}
}

func TestParseDotEnvRejectsMalformedLine(t *testing.T) {
	_, err := parseDotEnv("GOOD=1\nJUSTAKEY\n")
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("expected error naming line 2, got %v", err)
	}
}

func TestParseDotEnvStripsBOM(t *testing.T) {
	values, err := parseDotEnv("\ufeffDATABASE_URL=postgresql://x\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := values["DATABASE_URL"]; got != "postgresql://x" {
		t.Fatalf("got %q, want the BOM-stripped key to parse; values=%v", got, values)
	}
}

func TestParseDotEnvRejectsUTF16Content(t *testing.T) {
	utf16 := "D\x00A\x00T\x00A\x00"
	_, err := parseDotEnv(utf16)
	if err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("expected error asking for UTF-8, got %v", err)
	}
}

func TestLoadDotEnvSetsMissingVariables(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("NEOBLOG_TEST_FROM_FILE=file-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	const key = "NEOBLOG_TEST_FROM_FILE"
	original, hadOriginal := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			os.Setenv(key, original)
		} else {
			os.Unsetenv(key)
		}
	})

	if err := LoadDotEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "file-value" {
		t.Fatalf("got %q, want file-value", got)
	}
}

func TestLoadDotEnvDoesNotOverrideExistingVariables(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("NEOBLOG_TEST_KEEP=file-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	const key = "NEOBLOG_TEST_KEEP"
	t.Setenv(key, "env-value")

	if err := LoadDotEnv(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "env-value" {
		t.Fatalf("got %q, want env-value (environment must win over .env)", got)
	}
}

func TestLoadDotEnvMissingFileIsNotAnError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := LoadDotEnv(); err != nil {
		t.Fatalf("expected nil for missing .env, got %v", err)
	}
}

func TestLoadDotEnvFileReadsGivenPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom.env")
	if err := os.WriteFile(path, []byte("NEOBLOG_TEST_CUSTOM=value\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	const key = "NEOBLOG_TEST_CUSTOM"
	original, hadOriginal := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if hadOriginal {
			os.Setenv(key, original)
		} else {
			os.Unsetenv(key)
		}
	})

	if err := LoadDotEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv(key); got != "value" {
		t.Fatalf("got %q, want value", got)
	}
}

func TestLoadDotEnvFileMissingFileIsNotAnError(t *testing.T) {
	if err := LoadDotEnvFile(filepath.Join(t.TempDir(), "absent.env")); err != nil {
		t.Fatalf("expected nil for missing file, got %v", err)
	}
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	for _, key := range keys {
		original, had := os.LookupEnv(key)
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if had {
				_ = os.Setenv(key, original)
			} else {
				_ = os.Unsetenv(key)
			}
		})
	}
}

func TestEnsureDotEnvFileCreatesWhenAbsent(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")

	created, err := EnsureDotEnvFile(path)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !created {
		t.Fatal("expected the file to be created")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	values, err := parseDotEnv(string(data))
	if err != nil {
		t.Fatalf("generated file must parse: %v", err)
	}
	if strings.Contains(string(data), "#") {
		t.Fatalf("generated .env must carry plain key=value only, got: %q", data)
	}
	if len(values) != 4 {
		t.Fatalf("generated .env has %d keys, want 4: %q", len(values), data)
	}
	if values["DATABASE_URL"] == "" {
		t.Fatalf("generated file has no DATABASE_URL: %q", data)
	}
	if values["DATABASE_URL_TEST"] == "" {
		t.Fatalf("generated file has no DATABASE_URL_TEST: %q", data)
	}

	key, err := base64.StdEncoding.DecodeString(values["TOTP_ENCRYPTION_KEY"])
	if err != nil {
		t.Fatalf("TOTP_ENCRYPTION_KEY is not base64: %v", err)
	}
	if len(key) != totpKeyBytes {
		t.Fatalf("TOTP_ENCRYPTION_KEY decodes to %d bytes, want %d", len(key), totpKeyBytes)
	}
}

func TestEnsureDotEnvFileNeverOverwrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	original := []byte("DATABASE_URL=postgresql://mine/db\nTOTP_ENCRYPTION_KEY=keepme\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}

	created, err := EnsureDotEnvFile(path)
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if created {
		t.Fatal("an existing .env must not be reported as created")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != string(original) {
		t.Fatalf("existing .env was rewritten: %q", data)
	}

	again, err := EnsureDotEnvFile(path)
	if err != nil || again {
		t.Fatalf("second ensure = (%v, %v), want (false, nil)", again, err)
	}
}

func TestEnsuredDotEnvMakesLoadSucceed(t *testing.T) {
	t.Chdir(t.TempDir())
	unsetEnv(t, "DATABASE_URL", "DATABASE_URL_TEST", "TOTP_ENCRYPTION_KEY", "POSTGRES_PASSWORD", "APP_ENV")

	created, err := EnsureDotEnv()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !created {
		t.Fatal("expected .env to be created in the working directory")
	}

	if err := LoadDotEnv(); err != nil {
		t.Fatalf("load dotenv: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("config.Load after first-run bootstrap: %v", err)
	}
	if cfg.DatabaseURL == "" {
		t.Fatal("expected a database url from the generated file")
	}
	if len(cfg.TOTPEncryptionKey) != totpKeyBytes {
		t.Fatalf("TOTP key length = %d, want %d", len(cfg.TOTPEncryptionKey), totpKeyBytes)
	}
	if cfg.SMTPHost != "" {
		t.Fatalf("SMTP_HOST = %q, want empty so mail stays log-only", cfg.SMTPHost)
	}
}

func TestEnsureDotEnvSkipsProduction(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("APP_ENV", EnvironmentProduction)

	created, err := EnsureDotEnv()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if created {
		t.Fatal("production must not bootstrap a dev-shaped .env")
	}
	if _, err := os.Stat(filepath.Join(dir, ".env")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("a .env appeared in %s despite production environment: %v", dir, err)
	}
}

func TestEnsureDotEnvStillBootstrapsTestEnvironment(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("APP_ENV", EnvironmentTest)

	created, err := EnsureDotEnv()
	if err != nil {
		t.Fatalf("ensure: %v", err)
	}
	if !created {
		t.Fatal("test environment must keep the dev-shaped bootstrap so integration tests can run")
	}
}
