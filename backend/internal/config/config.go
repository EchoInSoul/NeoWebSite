package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"website/backend/internal/version"
)

type Config struct {
	Addr              string
	Environment       string
	Version           string
	ServiceName       string
	DatabaseURL       string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
}

func Load() (Config, error) {
	databaseURL := getEnv("DATABASE_URL", "")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	maxOpen, err := getEnvInt("DB_MAX_OPEN_CONNS", 10)
	if err != nil {
		return Config{}, err
	}

	maxIdle, err := getEnvInt("DB_MAX_IDLE_CONNS", 5)
	if err != nil {
		return Config{}, err
	}

	lifetime, err := getEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute)
	if err != nil {
		return Config{}, err
	}

	if maxIdle > maxOpen {
		return Config{}, fmt.Errorf("DB_MAX_IDLE_CONNS (%d) must not exceed DB_MAX_OPEN_CONNS (%d)", maxIdle, maxOpen)
	}

	return Config{
		Addr:              getEnv("ADDR", ":5732"),
		Environment:       getEnv("APP_ENV", "development"),
		Version:           getEnv("APP_VERSION", version.Version),
		ServiceName:       getEnv("SERVICE_NAME", "website-backend"),
		DatabaseURL:       databaseURL,
		DBMaxOpenConns:    maxOpen,
		DBMaxIdleConns:    maxIdle,
		DBConnMaxLifetime: lifetime,
	}, nil
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, value)
	}
	return parsed, nil
}

func getEnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(key)
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration, got %q", key, value)
	}
	return parsed, nil
}
