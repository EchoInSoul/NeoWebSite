package twofactor

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
	totpIssuer             = "两步验证"
	totpPeriodSeconds      = 30
	totpSecretBytes        = 10
	SkewWindows            = 1
	SetupReuseWindow       = 5 * time.Minute
	totpSetupClaimAttempts = 2
	recoveryCodeBytes      = 10
	recoveryCodeGroupSize  = 4
	RecoveryCodesPerBatch  = 5
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
		return totpSetup{}, fmt.Errorf("twofactor: generate totp key: %w", err)
	}
	return totpSetup{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}

func rebuildTOTPSetup(accountName, base32Secret string) (totpSetup, error) {
	raw, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(base32Secret)
	if err != nil {
		return totpSetup{}, fmt.Errorf("twofactor: decode totp secret: %w", err)
	}
	if len(raw) == 0 {
		return totpSetup{}, fmt.Errorf("twofactor: totp secret is empty")
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
		return totpSetup{}, fmt.Errorf("twofactor: rebuild totp key: %w", err)
	}
	return totpSetup{Secret: key.Secret(), OTPAuthURL: key.URL()}, nil
}

func MatchTOTPCounter(code string, base32Secret string, now time.Time) (int64, bool, error) {
	trimmed := strings.TrimSpace(code)
	if !IsSixDigitCode(trimmed) {
		return 0, false, nil
	}

	counter := now.Unix() / totpPeriodSeconds
	for offset := -int64(SkewWindows); offset <= int64(SkewWindows); offset++ {
		candidate := counter + offset
		expected, err := totp.GenerateCode(base32Secret, time.Unix(candidate*totpPeriodSeconds, 0))
		if err != nil {
			return 0, false, fmt.Errorf("twofactor: generate totp candidate: %w", err)
		}
		if subtle.ConstantTimeCompare([]byte(expected), []byte(trimmed)) == 1 {
			return candidate, true, nil
		}
	}
	return 0, false, nil
}

func IsSixDigitCode(code string) bool {
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

func GenerateRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, 0, count)
	seen := make(map[string]struct{}, count)

	for len(codes) < count {
		raw := make([]byte, recoveryCodeBytes)
		if _, err := rand.Read(raw); err != nil {
			return nil, fmt.Errorf("twofactor: generate recovery code: %w", err)
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

func HashRecoveryCode(code string) string {
	normalized := strings.ToLower(strings.TrimSpace(code))
	normalized = strings.ReplaceAll(normalized, "-", "")
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

const TestCodeHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

var TestTOTPKey = []byte("0123456789abcdef0123456789abcdef")
