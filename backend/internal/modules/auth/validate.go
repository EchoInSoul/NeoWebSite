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

func validPassword(password string) bool {
	if utf8.RuneCountInString(password) < 8 || len(password) > maxPasswordBytes {
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
