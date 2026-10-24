package mail

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	netmail "net/mail"
	"strings"

	"website/backend/internal/config"
)

const settingsGroup = "mail"

var errSMTPRequired = errors.New("mail: SMTP is required")

type Settings interface {
	GetGroup(ctx context.Context, group string) (map[string]any, error)
}

type settingsTransport struct {
	env      config.Config
	settings Settings
	logger   *slog.Logger
}

func newSettingsTransport(env config.Config, settings Settings, logger *slog.Logger) transport {
	return &settingsTransport{env: env, settings: settings, logger: logger}
}

func (t *settingsTransport) deliver(ctx context.Context, msg Message) error {
	cfg, send, err := resolveTransportConfig(ctx, t.env, t.settings)
	if err != nil {
		return err
	}
	if !send {
		if msg.RequireSMTP {
			return &deliveryError{err: errSMTPRequired, temporary: false}
		}
		return newLogSender(cfg, t.logger).deliver(ctx, msg)
	}
	return newSMTPTransport(cfg).deliver(ctx, msg)
}

func (t *settingsTransport) configured(ctx context.Context) (bool, error) {
	_, send, err := resolveTransportConfig(ctx, t.env, t.settings)
	if err != nil {
		return false, err
	}
	return send, nil
}

func resolveTransportConfig(ctx context.Context, env config.Config, settings Settings) (config.Config, bool, error) {
	if settings == nil {
		return env, env.SMTPHost != "", nil
	}

	values, err := settings.GetGroup(ctx, settingsGroup)
	if err != nil {
		return config.Config{}, false, fmt.Errorf("mail: read settings: %w", err)
	}

	host, err := settingsString(values, "smtp_host")
	if err != nil {
		return config.Config{}, false, err
	}
	port, err := settingsNumber(values, "smtp_port")
	if err != nil {
		return config.Config{}, false, err
	}
	if port < 1 || port > 65535 {
		return config.Config{}, false, fmt.Errorf("mail: smtp_port %d is out of range", port)
	}
	encryption, err := settingsString(values, "smtp_encryption")
	if err != nil {
		return config.Config{}, false, err
	}
	username, err := settingsString(values, "smtp_username")
	if err != nil {
		return config.Config{}, false, err
	}
	password, err := settingsString(values, "smtp_password")
	if err != nil {
		return config.Config{}, false, err
	}
	sender, err := settingsString(values, "sender_address")
	if err != nil {
		return config.Config{}, false, err
	}
	senderName, err := settingsString(values, "sender_name")
	if err != nil {
		return config.Config{}, false, err
	}
	enabled, ok := values["enabled"].(bool)
	if !ok {
		return config.Config{}, false, fmt.Errorf("mail: setting %s.enabled is not a bool", settingsGroup)
	}

	cfg := env
	cfg.SMTPHost = strings.TrimSpace(host)
	cfg.SMTPPort = int(port)
	cfg.SMTPUsername = strings.TrimSpace(username)
	cfg.SMTPPassword = password
	cfg.SMTPFrom = strings.TrimSpace(sender)
	cfg.SMTPFromName = senderName

	switch encryption {
	case "ssl":
		cfg.SMTPTLS = config.SMTPTLSSSL
	case "starttls":
		cfg.SMTPTLS = config.SMTPTLSStartTLS
	default:
		return config.Config{}, false, fmt.Errorf("mail: smtp_encryption %q is not supported", encryption)
	}

	if !enabled {
		return cfg, false, nil
	}
	if cfg.SMTPHost == "" {
		if env.SMTPHost == "" {
			return cfg, false, nil
		}
		if err := validateEffectiveConfig(env); err != nil {
			return config.Config{}, false, err
		}
		return env, true, nil
	}
	if err := validateEffectiveConfig(cfg); err != nil {
		return config.Config{}, false, err
	}
	return cfg, true, nil
}

func validateEffectiveConfig(cfg config.Config) error {
	if cfg.SMTPFrom == "" {
		return fmt.Errorf("mail: sender_address is required when smtp_host is set")
	}
	address, err := netmail.ParseAddress(cfg.SMTPFrom)
	if err != nil || address.Name != "" || address.Address != cfg.SMTPFrom {
		return fmt.Errorf("mail: sender_address %q is invalid", cfg.SMTPFrom)
	}
	if cfg.SMTPUsername != "" && cfg.SMTPPassword == "" {
		return fmt.Errorf("mail: smtp_password is required when smtp_username is set")
	}
	return nil
}

func settingsString(values map[string]any, name string) (string, error) {
	value, ok := values[name].(string)
	if !ok {
		return "", fmt.Errorf("mail: setting %s.%s is not a string", settingsGroup, name)
	}
	return value, nil
}

func settingsNumber(values map[string]any, name string) (int64, error) {
	value, ok := values[name].(int64)
	if !ok {
		return 0, fmt.Errorf("mail: setting %s.%s is not a number", settingsGroup, name)
	}
	return value, nil
}
