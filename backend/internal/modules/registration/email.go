package registration

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"website/backend/internal/modules/mail"
)

func (s *Service) registrationEmailData(ctx context.Context, nickname string) (map[string]string, error) {
	site, err := s.settings.GetGroup(ctx, "site")
	if err != nil {
		return nil, err
	}
	title, ok := site["title"].(string)
	if !ok {
		return nil, errors.New("registration: site.title is not a string")
	}
	rawURL, ok := site["url"].(string)
	if !ok {
		return nil, errors.New("registration: site.url is not a string")
	}
	base, err := mail.SiteURL(rawURL)
	if err != nil {
		return nil, err
	}
	return map[string]string{
		"siteTitle":      title,
		"siteUrl":        base,
		"username":       nickname,
		"expiresMinutes": fmt.Sprintf("%d", int(pendingRegistrationTTL/time.Minute)),
		"sentAt":         time.Now().Format("2006-01-02 15:04:05"),
		"code":           "",
		"verifyUrl":      "",
	}, nil
}

func (s *Service) compensatePending(ctx context.Context, record pendingRegistrationRecord) {
	if _, err := s.repo.deletePendingRegistration(ctx, record.ID, record.TokenHash); err != nil {
		slog.Error("registration: delete pending registration", "err", err)
	}
}
