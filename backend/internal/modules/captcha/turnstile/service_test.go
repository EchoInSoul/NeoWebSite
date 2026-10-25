package turnstile

import (
	"context"
	"errors"
	"testing"
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

type stubSettings map[string]string

func (s stubSettings) GetString(_ context.Context, group, name string) (string, error) {
	return s[group+"."+name], nil
}

type stubVerifier struct {
	outcome Outcome
	calls   int
}

func (v *stubVerifier) Verify(context.Context, string, string, string) Outcome {
	v.calls++
	return v.outcome
}

func newLimitTestService(outcome Outcome, limit int) (*Service, *stubVerifier) {
	settings := stubSettings{
		"captcha.turnstile_secret_key": "secret",
	}
	verifier := &stubVerifier{outcome: outcome}
	service := NewServiceWithPolicies(settings, verifier, ratelimit.New(), Policies{
		Failure: ratelimit.FailurePolicy{
			Limit:    limit,
			Window:   time.Minute,
			Cooldown: time.Minute,
		},
	})
	return service, verifier
}

func TestVerifyRateLimitsFailuresPerIP(t *testing.T) {
	service, verifier := newLimitTestService(Passed, 1)
	ctx := context.Background()

	if err := service.Verify(ctx, "login", "", "203.0.113.9"); !errors.Is(err, apierror.ErrCaptchaFailed) {
		t.Fatalf("empty token error = %v, want captcha_failed", err)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls after empty token = %d, want 0", verifier.calls)
	}

	err := service.Verify(ctx, "login", "token", "203.0.113.9")
	if !errors.Is(err, apierror.ErrRateLimit) {
		t.Fatalf("limited error = %v, want rate_limited", err)
	}
	if verifier.calls != 0 {
		t.Fatalf("verifier calls while limited = %d, want 0", verifier.calls)
	}

	if err := service.Verify(ctx, "login", "token", "198.51.100.7"); err != nil {
		t.Fatalf("other IP Verify = %v, want nil", err)
	}
	if verifier.calls != 1 {
		t.Fatalf("verifier calls after other IP = %d, want 1", verifier.calls)
	}
}

func TestVerifyDoesNotCountUnavailable(t *testing.T) {
	service, verifier := newLimitTestService(Unavailable, 1)
	ctx := context.Background()

	if err := service.Verify(ctx, "login", "token", "203.0.113.9"); err == nil {
		t.Fatal("unavailable Verify = nil, want captcha_failed")
	}
	if verifier.calls != 1 {
		t.Fatalf("verifier calls = %d, want 1", verifier.calls)
	}

	verifier.outcome = Passed
	if err := service.Verify(ctx, "login", "token", "203.0.113.9"); err != nil {
		t.Fatalf("Verify after unavailable = %v, want nil", err)
	}
}
