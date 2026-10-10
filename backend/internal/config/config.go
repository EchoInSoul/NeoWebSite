package config

import (
	"os"

	"website/backend/internal/version"
)

type Config struct {
	Addr        string
	Environment string
	Version     string
	ServiceName string
}

func Load() Config {
	return Config{
		Addr:        getEnv("ADDR", ":5732"),
		Environment: getEnv("APP_ENV", "development"),
		Version:     getEnv("APP_VERSION", version.Version),
		ServiceName: getEnv("SERVICE_NAME", "website-backend"),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
