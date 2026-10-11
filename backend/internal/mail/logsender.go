package mail

import (
	"context"
	"log/slog"

	"website/backend/internal/config"
)

const defaultLogSenderFrom = "noreply@localhost"

type logSender struct {
	from     string
	fromName string
	logger   *slog.Logger
}

func newLogSender(cfg config.Config, logger *slog.Logger) *logSender {
	from := cfg.SMTPFrom
	if from == "" {
		from = defaultLogSenderFrom
	}
	return &logSender{from: from, fromName: cfg.SMTPFromName, logger: logger}
}

func (s *logSender) deliver(_ context.Context, msg Message) error {
	s.logger.Info("mail not sent: smtp is not configured",
		"to", msg.To,
		"from", s.from,
		"from_name", s.fromName,
		"subject", msg.Subject,
		"text", msg.Body.Text,
	)
	return nil
}
