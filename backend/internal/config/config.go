package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"website/backend/internal/version"
)

type SMTPTLSMode string

const (
	SMTPTLSStartTLS SMTPTLSMode = "starttls"
	SMTPTLSSSL      SMTPTLSMode = "ssl"
)

const (
	EnvironmentDevelopment = "development"
	EnvironmentProduction  = "production"
)

const DefaultEnvironment = EnvironmentProduction

const (
	defaultDBMaxOpenConns    = 10
	defaultDBMaxIdleConns    = 5
	defaultDBConnMaxLifetime = 30 * time.Minute
	defaultSessionLifetime   = 168 * time.Hour
	defaultSMTPFromName      = ""
	maxSMTPPort              = 65535
	totpKeyBytes             = 32
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
	SessionLifetime   time.Duration
	SMTPHost          string
	SMTPPort          int
	SMTPTLS           SMTPTLSMode
	SMTPUsername      string
	SMTPPassword      string
	SMTPFrom          string
	SMTPFromName      string
	TOTPEncryptionKey []byte
}

func (c Config) IsProduction() bool {
	return c.Environment == EnvironmentProduction
}

func Load() (Config, error) {
	databaseURL := getEnv("DATABASE_URL", "")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required (add it to .env or export it)")
	}

	environment, err := EffectiveEnvironment()
	if err != nil {
		return Config{}, err
	}

	maxOpen := defaultDBMaxOpenConns
	maxIdle := defaultDBMaxIdleConns
	lifetime := defaultDBConnMaxLifetime
	sessionLifetime := defaultSessionLifetime

	smtp, err := loadSMTP()
	if err != nil {
		return Config{}, err
	}

	totpKey, err := loadTOTPKey()
	if err != nil {
		return Config{}, err
	}

	return Config{
		Addr:              getEnv("ADDR", ":5732"),
		Environment:       environment,
		Version:           getEnv("APP_VERSION", version.Version),
		ServiceName:       getEnv("SERVICE_NAME", "website-backend"),
		DatabaseURL:       databaseURL,
		DBMaxOpenConns:    maxOpen,
		DBMaxIdleConns:    maxIdle,
		DBConnMaxLifetime: lifetime,
		SessionLifetime:   sessionLifetime,
		SMTPHost:          smtp.Host,
		SMTPPort:          smtp.Port,
		SMTPTLS:           smtp.TLS,
		SMTPUsername:      smtp.Username,
		SMTPPassword:      smtp.Password,
		SMTPFrom:          smtp.From,
		SMTPFromName:      smtp.FromName,
		TOTPEncryptionKey: totpKey,
	}, nil
}

type smtpConfig struct {
	Host     string
	Port     int
	TLS      SMTPTLSMode
	Username string
	Password string
	From     string
	FromName string
}

func EffectiveEnvironment() (string, error) {
	value := DefaultEnvironment
	if v, ok := os.LookupEnv("APP_ENV"); ok {
		value = strings.TrimSpace(v)
	}
	switch value {
	case EnvironmentDevelopment, EnvironmentProduction:
		return value, nil
	}
	return "", fmt.Errorf("APP_ENV must be one of %q or %q, got %q", EnvironmentDevelopment, EnvironmentProduction, value)
}

func loadSMTP() (smtpConfig, error) {
	host := os.Getenv("SMTP_HOST")
	if host == "" {
		return smtpConfig{
			FromName: getEnv("SMTP_FROM_NAME", defaultSMTPFromName),
		}, nil
	}

	port, err := requireEnvInt("SMTP_PORT")
	if err != nil {
		return smtpConfig{}, err
	}
	if port > maxSMTPPort {
		return smtpConfig{}, fmt.Errorf("SMTP_PORT must be at most %d, got %d", maxSMTPPort, port)
	}

	tlsValue, err := requireEnv("SMTP_TLS")
	if err != nil {
		return smtpConfig{}, err
	}
	tlsMode := SMTPTLSMode(tlsValue)
	if tlsMode != SMTPTLSStartTLS && tlsMode != SMTPTLSSSL {
		return smtpConfig{}, fmt.Errorf("SMTP_TLS must be %q or %q, got %q", SMTPTLSStartTLS, SMTPTLSSSL, tlsMode)
	}

	username := os.Getenv("SMTP_USERNAME")
	password := os.Getenv("SMTP_PASSWORD")
	if username != "" && password == "" {
		return smtpConfig{}, errors.New("SMTP_PASSWORD is required when SMTP_USERNAME is set")
	}

	from := os.Getenv("SMTP_FROM")
	if from == "" {
		return smtpConfig{}, errors.New("SMTP_FROM is required when SMTP_HOST is set")
	}

	return smtpConfig{
		Host:     host,
		Port:     port,
		TLS:      tlsMode,
		Username: username,
		Password: password,
		From:     from,
		FromName: getEnv("SMTP_FROM_NAME", defaultSMTPFromName),
	}, nil
}

func loadTOTPKey() ([]byte, error) {
	value := strings.TrimSpace(os.Getenv("TOTP_ENCRYPTION_KEY"))
	if value == "" {
		return nil, errors.New("TOTP_ENCRYPTION_KEY is required (generate one with: openssl rand -base64 32)")
	}

	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return nil, errors.New("TOTP_ENCRYPTION_KEY must be valid base64 (generate one with: openssl rand -base64 32)")
	}
	if len(key) != totpKeyBytes {
		return nil, fmt.Errorf("TOTP_ENCRYPTION_KEY must decode to %d bytes, got %d", totpKeyBytes, len(key))
	}
	return key, nil
}

func getEnv(key string, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func requireEnv(key string) (string, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return value, nil
}

func requireEnvInt(key string) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, fmt.Errorf("%s is required", key)
	}

	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, value)
	}
	return parsed, nil
}
