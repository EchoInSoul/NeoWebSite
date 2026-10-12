package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	totpIssuer        = "NeoBlog"
	totpPeriodSeconds = 30
	totpSkewWindows   = 1
	recoveryCodeBytes = 16
)

var totpNow = time.Now

type totpSetup struct {
	Secret     string
	OTPAuthURL string
}

func generateTOTPSetup(accountName string) (totpSetup, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: accountName,
		Period:      totpPeriodSeconds,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return totpSetup{}, fmt.Errorf("generate totp key: %w", err)
	}
	return totpSetup{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}

func matchTOTPCounter(code string, base32Secret string, now time.Time) (int64, bool, error) {
	trimmed := strings.TrimSpace(code)
	if !isSixDigitCode(trimmed) {
		return 0, false, nil
	}

	counter := now.Unix() / totpPeriodSeconds
	for offset := -int64(totpSkewWindows); offset <= int64(totpSkewWindows); offset++ {
		candidate := counter + offset
		expected, err := totp.GenerateCode(base32Secret, time.Unix(candidate*totpPeriodSeconds, 0))
		if err != nil {
			return 0, false, fmt.Errorf("generate totp candidate: %w", err)
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(trimmed)) == 1 {
			return candidate, true, nil
		}
	}
	return 0, false, nil
}

func isSixDigitCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func generateRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, 0, count)
	seen := make(map[string]struct{}, count)

	for len(codes) < count {
		raw := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("generate recovery code: %w", err)
		}

		code := hex.EncodeToString(raw)
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes, nil
}

func hashRecoveryCode(code string) string {
	normalized := strings.ToLower(strings.TrimSpace(code))
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
