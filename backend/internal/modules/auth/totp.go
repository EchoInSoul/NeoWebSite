package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	totpIssuer             = "NeoBlog"
	totpPeriodSeconds      = 30
	totpSecretBytes        = 10
	totpSkewWindows        = 1
	totpSetupReuseWindow   = 5 * time.Minute
	totpSetupClaimAttempts = 2
	recoveryCodeBytes      = 10
	recoveryCodeGroupSize  = 4
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
		SecretSize:  totpSecretBytes,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return totpSetup{}, fmt.Errorf("auth: generate totp key: %w", err)
	}
	return totpSetup{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}

func rebuildTOTPSetup(accountName, base32Secret string) (totpSetup, error) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(base32Secret)
	if err != nil {
		return totpSetup{}, fmt.Errorf("auth: decode totp secret: %w", err)
	}
	if len(raw) == 0 {
		return totpSetup{}, fmt.Errorf("auth: totp secret is empty")
	}
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      totpIssuer,
		AccountName: accountName,
		Period:      totpPeriodSeconds,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1,
		Secret:      raw,
	})
	if err != nil {
		return totpSetup{}, fmt.Errorf("auth: rebuild totp key: %w", err)
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
			return 0, false, fmt.Errorf("auth: generate totp candidate: %w", err)
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
			return nil, fmt.Errorf("auth: generate recovery code: %w", err)
		}

		encoded := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
		code := formatRecoveryCode(encoded)
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes, nil
}

func formatRecoveryCode(encoded string) string {
	groups := make([]string, 0, len(encoded)/recoveryCodeGroupSize)
	for start := 0; start < len(encoded); start += recoveryCodeGroupSize {
		groups = append(groups, encoded[start:min(start+recoveryCodeGroupSize, len(encoded))])
	}
	return strings.Join(groups, "-")
}

func hashRecoveryCode(code string) string {
	normalized := strings.ToLower(strings.TrimSpace(code))
	normalized = strings.ReplaceAll(normalized, "-", "")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}
