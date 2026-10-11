package config

import (
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
