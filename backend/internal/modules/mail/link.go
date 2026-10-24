package mail

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func SiteURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("mail: site url is empty")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("mail: parse site url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("mail: site url scheme %q is not supported", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return "", errors.New("mail: site url has no host")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("mail: site url must not contain query or fragment")
	}

	return parsed.Scheme + "://" + parsed.Host + strings.TrimRight(parsed.Path, "/"), nil
}
