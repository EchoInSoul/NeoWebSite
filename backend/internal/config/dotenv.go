package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
)

func LoadDotEnv() error {
	data, err := os.ReadFile(".env")
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	values, err := parseDotEnv(string(data))
	if err != nil {
		return fmt.Errorf(".env: %w", err)
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
