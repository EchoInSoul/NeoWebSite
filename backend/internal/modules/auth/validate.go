package auth

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

func normalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func normalizeNickname(raw string) string {
	return strings.TrimSpace(raw)
}

func normalizeBio(raw string) string {
	return strings.TrimSpace(raw)
}

func validEmail(email string) bool {
	if len(email) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

func validNickname(nickname string) bool {
	count := utf8.RuneCountInString(nickname)
	if count < 2 || count > 32 {
		return false
	}
	if strings.Contains(nickname, "@") {
		return false
	}
	return !strings.ContainsFunc(nickname, unicode.IsSpace)
}

func validBio(bio string) bool {
	return utf8.RuneCountInString(bio) <= 1000
}

const (
	passwordBaselineMinLength = 8
	passwordMinLengthMax      = 64
)

func validPassword(password string, minLength int) bool {
	if utf8.RuneCountInString(password) < minLength || len(password) > maxPasswordBytes {
		return false
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			hasLetter = true
		case r >= '0' && r <= '9':
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

func effectivePasswordMinLength(requirementsEnabled bool, configured int64) int {
	if !requirementsEnabled {
		return passwordBaselineMinLength
	}
	if configured < passwordBaselineMinLength {
		return passwordBaselineMinLength
	}
	if configured > passwordMinLengthMax {
		return passwordMinLengthMax
	}
	return int(configured)
}

func emailDomainAllowed(email string, allowed []string) bool {
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, entry := range allowed {
		if strings.EqualFold(entry, domain) {
			return true
		}
	}
	return false
}
