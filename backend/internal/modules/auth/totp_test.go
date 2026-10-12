package auth

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

const testTOTPSecret = "JBSWY3DPEHPK3PXP"

func codeAt(t *testing.T, secret string, counter int64) string {
	t.Helper()

	code, err := totp.GenerateCode(secret, time.Unix(counter*totpPeriodSeconds, 0))
	if err != nil {
		t.Fatalf("generate code at window %d: %v", counter, err)
	}
	return code
}

func pinWindow(t *testing.T, secret string, counter int64) string {
	t.Helper()

	at := time.Unix(counter*totpPeriodSeconds+totpPeriodSeconds/2, 0)
	previous := totpNow
	totpNow = func() time.Time { return at }
	t.Cleanup(func() { totpNow = previous })

	return codeAt(t, secret, counter)
}

func TestMatchTOTPCounterCurrentWindow(t *testing.T) {
	now := time.Unix(1700000001, 0)
	want := now.Unix() / totpPeriodSeconds

	code, err := totp.GenerateCode(testTOTPSecret, now)
	if err != nil {
		t.Fatalf("generate code: %v", err)
	}

	counter, matched, err := matchTOTPCounter(code, testTOTPSecret, now)
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

		counter, matched, err := matchTOTPCounter(code, testTOTPSecret, now)
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

		counter, matched, err := matchTOTPCounter(code, testTOTPSecret, now)
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
		counter, matched, err := matchTOTPCounter(code, testTOTPSecret, now)
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

	counter, matched, err := matchTOTPCounter("123456", "!!!!", now)
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

func TestGenerateRecoveryCodesShape(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9a-f]{32}$`)

	codes, err := generateRecoveryCodes(10)
	if err != nil {
		t.Fatalf("generate recovery codes: %v", err)
	}
	if len(codes) != 10 {
		t.Fatalf("codes = %d, want 10", len(codes))
	}

	seen := make(map[string]struct{}, len(codes))
	for _, code := range codes {
		if !pattern.MatchString(code) {
			t.Fatalf("code %q does not match 32 lowercase hex", code)
		}
		if _, ok := seen[code]; ok {
			t.Fatalf("duplicate code %q in batch", code)
		}
		seen[code] = struct{}{}
	}
}

func TestHashRecoveryCodeNormalizes(t *testing.T) {
	hashed := hashRecoveryCode(" ABcd ")
	if len(hashed) != 64 {
		t.Fatalf("hash length = %d, want 64", len(hashed))
	}
	if hashed != hashRecoveryCode("abcd") {
		t.Fatalf("normalization differs: %q vs %q", hashed, hashRecoveryCode("abcd"))
	}
	if hashed == hashRecoveryCode("abcde") {
		t.Fatal("different codes hashed identically")
	}
	if strings.Contains(hashed, " ") {
		t.Fatalf("hash %q contains whitespace", hashed)
	}
}
