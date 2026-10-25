package userinput

import (
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	PasswordMaxBytes          = 72
	PasswordBaselineMinLength = 8
	PasswordMinLengthMax      = 64
)

func NormalizeEmail(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func NormalizeNickname(raw string) string {
	return strings.TrimSpace(raw)
}

func NormalizeBio(raw string) string {
	return strings.TrimSpace(raw)
}

func ValidEmail(email string) bool {
	if len(email) > 254 {
		return false
	}
	addr, err := mail.ParseAddress(email)
	return err == nil && addr.Address == email
}

func ValidNickname(nickname string) bool {
	count := utf8.RuneCountInString(nickname)
	if count < 2 || count > 32 {
		return false
	}
	if strings.Contains(nickname, "@") {
		return false
	}
	return !strings.ContainsFunc(nickname, unicode.IsSpace)
}

func ValidBio(bio string) bool {
	return utf8.RuneCountInString(bio) <= 1000
}

func ValidPassword(password string, minLength int) bool {
	if utf8.RuneCountInString(password) < minLength || len(password) > PasswordMaxBytes {
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

func EffectivePasswordMinLength(requirementsEnabled bool, configured int64) int {
	if !requirementsEnabled {
		return PasswordBaselineMinLength
	}
	if configured < PasswordBaselineMinLength {
		return PasswordBaselineMinLength
	}
	if configured > PasswordMinLengthMax {
		return PasswordMinLengthMax
	}
	return int(configured)
}

func EmailDomainAllowed(email string, allowed []string) bool {
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
