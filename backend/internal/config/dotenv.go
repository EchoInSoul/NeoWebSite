package config

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

const dotEnvPath = ".env"

const generatedDotEnvTemplate = `DATABASE_URL=postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog?sslmode=disable
DATABASE_URL_TEST=postgresql://neoblog:neoblog@127.0.0.1:5434/neoblog_test?sslmode=disable
POSTGRES_PASSWORD=neoblog
TOTP_ENCRYPTION_KEY=%s
`

func LoadDotEnv() error {
	return LoadDotEnvFile(dotEnvPath)
}

func EnsureDotEnv() (bool, error) {
	return EnsureDotEnvFile(dotEnvPath)
}

func EnsureDotEnvFile(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return false, fmt.Errorf("dotenv: stat %s: %w", path, err)
	}

	environment, err := EffectiveEnvironment()
	if err != nil {
		return false, fmt.Errorf("dotenv: %w", err)
	}
	if environment == EnvironmentProduction {
		return false, nil
	}

	key, err := generateTOTPEncryptionKey()
	if err != nil {
		return false, err
	}

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return false, nil
		}
		return false, fmt.Errorf("dotenv: create %s: %w", path, err)
	}

	if _, err := file.WriteString(fmt.Sprintf(generatedDotEnvTemplate, key)); err != nil {
		_ = file.Close()
		return false, fmt.Errorf("dotenv: write %s: %w", path, err)
	}

	if err := file.Close(); err != nil {
		return false, fmt.Errorf("dotenv: close %s: %w", path, err)
	}

	return true, nil
}

func generateTOTPEncryptionKey() (string, error) {
	buf := make([]byte, totpKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("dotenv: generate TOTP key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func LoadDotEnvFile(path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	values, err := parseDotEnv(string(data))
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	for key, value := range values {
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}

	return nil
}

func parseDotEnv(content string) (map[string]string, error) {
	content = strings.TrimPrefix(content, "\ufeff")
	if strings.ContainsRune(content, 0) {
		return nil, errors.New("file is not UTF-8 text (looks like UTF-16); save it as UTF-8")
	}

	values := make(map[string]string)

	for index, raw := range strings.Split(content, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		line = strings.TrimPrefix(line, "export ")

		key, value, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("line %d: missing '='", index+1)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", index+1)
		}

		value = strings.TrimSpace(value)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}

		values[key] = value
	}

	return values, nil
}
