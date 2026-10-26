package mail

import (
	"context"
	"errors"
)

func SiteInfo(ctx context.Context, settings Settings) (string, string, error) {
	site, err := settings.GetGroup(ctx, "site")
	if err != nil {
		return "", "", err
	}
	title, ok := site["title"].(string)
	if !ok {
		return "", "", errors.New("mail: site.title is not a string")
	}
	rawURL, ok := site["url"].(string)
	if !ok {
		return "", "", errors.New("mail: site.url is not a string")
	}
	base, err := SiteURL(rawURL)
	if err != nil {
		return "", "", err
	}
	return title, base, nil
}
