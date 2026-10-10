package config

import "os"

type Config struct {
	Addr        string
	Environment string
	Version     string
	ServiceName string
}

func Load() Config {
	return Config{
		Addr:        getEnv("ADDR", ":8080"),
		Environment: getEnv("APP_ENV", "development"),
		Version:     getEnv("APP_VERSION", "dev"),
		ServiceName: getEnv("SERVICE_NAME", "website-backend"),
	}
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
