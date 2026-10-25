package twofactor

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func CodeAt(t *testing.T, secret string, counter int64) string {
	t.Helper()

	code, err := totp.GenerateCode(secret, time.Unix(counter*totpPeriodSeconds, 0))
	if err != nil {
		t.Fatalf("generate code at window %d: %v", counter, err)
	}
	return code
}

func PinWindow(t *testing.T, secret string, counter int64) string {
	t.Helper()

	at := time.Unix(counter*totpPeriodSeconds+totpPeriodSeconds/2, 0)
	previous := totpNow
	totpNow = func() time.Time { return at }
	t.Cleanup(func() { totpNow = previous })

	return CodeAt(t, secret, counter)
}

func CurrentWindow() int64 {
	return totpNow().Unix() / totpPeriodSeconds
}

func FakeTOTPClock(t *testing.T, base time.Time) func(time.Time) {
	t.Helper()
	previous := totpNow
	now := base
	totpNow = func() time.Time { return now }
	t.Cleanup(func() { totpNow = previous })
	return func(at time.Time) { now = at }
}
