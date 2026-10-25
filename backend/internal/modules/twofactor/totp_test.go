package twofactor

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

const testTOTPSecret = "JBSWY3DPEHPK3PXP"

func TestMatchTOTPCounterCurrentWindow(t *testing.T) {
	now := time.Unix(1700000001, 0)
	want := now.Unix() / totpPeriodSeconds

	code, err := totp.GenerateCode(testTOTPSecret, now)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	counter, matched, err := MatchTOTPCounter(code, testTOTPSecret, now)
	if err != nil {
		t.Fatalf("match: %v", err)
	}
	if !matched {
		t.Fatal("current window code did not match")
	}
	if counter != want {
		t.Fatalf("counter = %d, want %d", counter, want)
	}
}

func TestMatchTOTPCounterAdjacentWindows(t *testing.T) {
	now := time.Unix(1700000001, 0)
	current := now.Unix() / totpPeriodSeconds

	for _, offset := range []int64{-1, 1} {
		at := now.Add(time.Duration(offset*totpPeriodSeconds) * time.Second)
		code, err := totp.GenerateCode(testTOTPSecret, at)
		if err != nil {
			t.Fatalf("generate code at %d: %v", offset, err)
		}

		counter, matched, err := MatchTOTPCounter(code, testTOTPSecret, now)
		if err != nil {
			t.Fatalf("match at offset %d: %v", offset, err)
		}
		if !matched {
			t.Fatalf("adjacent window %d did not match", offset)
		}
		if counter != current+offset {
			t.Fatalf("counter = %d, want %d", counter, current+offset)
		}
	}
}

func TestMatchTOTPCounterRejectsFarWindow(t *testing.T) {
	now := time.Unix(1700000001, 0)

	for _, offset := range []int64{-3, 3} {
		at := now.Add(time.Duration(offset*totpPeriodSeconds) * time.Second)
		code, err := totp.GenerateCode(testTOTPSecret, at)
		if err != nil {
			t.Fatalf("generate code at %d: %v", offset, err)
		}

		counter, matched, err := MatchTOTPCounter(code, testTOTPSecret, now)
		if err != nil {
			t.Fatalf("match at offset %d: %v", offset, err)
		}
		if matched {
			t.Fatalf("far window %d matched, want rejected (counter %d)", offset, counter)
		}
		if counter != 0 {
			t.Fatalf("counter = %d, want 0 when unmatched", counter)
		}
	}
}

func TestMatchTOTPCounterRejectsGarbage(t *testing.T) {
	now := time.Unix(1700000001, 0)

	for _, code := range []string{"12345", "abcdef", "", "1234567"} {
		counter, matched, err := MatchTOTPCounter(code, testTOTPSecret, now)
		if err != nil {
			t.Fatalf("match %q returned error %v, want nil", code, err)
		}
		if matched {
			t.Fatalf("match %q = true, want false", code)
		}
		if counter != 0 {
			t.Fatalf("counter = %d, want 0", counter)
		}
	}
}

func TestMatchTOTPCounterRejectsInvalidSecret(t *testing.T) {
	now := time.Unix(1700000001, 0)

	counter, matched, err := MatchTOTPCounter("123456", "!!!!", now)
	if err == nil {
		t.Fatal("invalid base32 secret returned nil error, want error")
	}
	if matched {
		t.Fatal("invalid secret matched, want false")
	}
	if counter != 0 {
		t.Fatalf("counter = %d, want 0", counter)
	}
}

func TestGenerateTOTPSetupSecretShape(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Z2-7]{16}$`)

	setup, err := generateTOTPSetup("user@example.com")
	if err != nil {
		t.Fatalf("generate setup: %v", err)
	}
	if !pattern.MatchString(setup.Secret) {
		t.Fatalf("secret %q is not 16 base32 chars", setup.Secret)
	}
	if !strings.Contains(setup.OTPAuthURL, "secret="+setup.Secret) {
		t.Fatalf("otpauth url %q does not carry secret %q", setup.OTPAuthURL, setup.Secret)
	}
}

func TestRecoveryCodeBatchSize(t *testing.T) {
	if RecoveryCodesPerBatch != 5 {
		t.Fatalf("RecoveryCodesPerBatch = %d, want 5", RecoveryCodesPerBatch)
	}
}

func TestGenerateRecoveryCodesShape(t *testing.T) {
	pattern := regexp.MustCompile(`^[A-Z2-7]{4}(-[A-Z2-7]{4}){3}$`)

	codes, err := GenerateRecoveryCodes(10)
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	if len(codes) != 10 {
		t.Fatalf("codes = %d, want 10", len(codes))
	}

	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if !pattern.MatchString(code) {
			t.Fatalf("code %q is not 4 groups of 4 uppercase base32", code)
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("duplicate code %q in batch", code)
		}
		seen[code] = struct{}{}
	}
}

func TestHashRecoveryCodeNormalizes(t *testing.T) {
	hashed := HashRecoveryCode(" ABcd ")
	if len(hashed) != 64 {
		t.Fatalf("hash length = %d, want 64", len(hashed))
	}
	if hashed != HashRecoveryCode("abcd") {
		t.Fatalf("normalization differs: %q vs %q", hashed, HashRecoveryCode("abcd"))
	}
	if hashed != HashRecoveryCode("AB-CD") {
		t.Fatalf("hyphen stripping differs: %q vs %q", hashed, HashRecoveryCode("AB-CD"))
	}
	if hashed == HashRecoveryCode("abcde") {
		t.Fatal("different codes hashed identically")
	}
	if strings.Contains(hashed, " ") {
		t.Fatalf("hash %q contains whitespace", hashed)
	}
}
