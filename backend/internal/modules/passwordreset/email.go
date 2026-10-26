package passwordreset

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"time"

	"website/backend/internal/database/ent"
	"website/backend/internal/modules/mail"
)

func (s *Service) siteInfo(ctx context.Context) (string, string, error) {
	site, err := s.settings.GetGroup(ctx, "site")
	if err != nil {
		return "", "", err
	}
	title, ok := site["title"].(string)
	if !ok {
		return "", "", errors.New("passwordreset: site.title is not a string")
	}
	rawURL, ok := site["url"].(string)
	if !ok {
		return "", "", errors.New("passwordreset: site.url is not a string")
	}
	base, err := mail.SiteURL(rawURL)
	if err != nil {
		return "", "", err
	}
	return title, base, nil
}

func resetEmailData(title, baseURL string, user *ent.User, rawToken string) map[string]string {
	return map[string]string{
		"siteTitle":      title,
		"siteUrl":        baseURL,
		"username":       user.Nickname,
		"expiresMinutes": fmt.Sprintf("%d", int(resetTTL/time.Minute)),
		"sentAt":         time.Now().Format("2006-01-02 15:04:05"),
		"resetUrl":       baseURL + "/reset-password/confirm?token=" + url.QueryEscape(rawToken),
		"code":           "",
	}
}
