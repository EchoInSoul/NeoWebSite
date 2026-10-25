package builtin

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
)

func TestChallengeAndSolveFlow(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	if !strings.HasPrefix(challenge.MasterImage, "data:image/jpeg;base64,") {
		t.Fatalf("master image = %.40q", challenge.MasterImage)
	}
	if !strings.HasPrefix(challenge.TileImage, "data:image/png;base64,") {
		t.Fatalf("tile image = %.40q", challenge.TileImage)
	}
	expiresAt, err := time.Parse(time.RFC3339, challenge.ExpiresAt)
	if err != nil {
		t.Fatalf("expires_at = %q: %v", challenge.ExpiresAt, err)
	}
	if !expiresAt.After(time.Now()) {
		t.Fatalf("expires_at = %v, want future", expiresAt)
	}

	x, y := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, x, y)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(solved.CaptchaToken)
	if err != nil {
		t.Fatalf("token not base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("token bytes = %d, want 32", len(raw))
	}
}

func TestChallengeRejectedWhenNotActive(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	cases := []struct {
		name     string
		enabled  bool
		provider string
	}{
		{"disabled", false, "builtin"},
		{"turnstile", true, "turnstile"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setCaptchaSettings(t, settingsSvc, tc.enabled, tc.provider)
			_, err := service.Challenge(ctx, "login", "1.2.3.4")
			assertErrorCode(t, err, "captcha_disabled")
		})
	}
}

func TestChallengeRejectsUnknownScope(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	_, err := service.Challenge(context.Background(), "signup", "1.2.3.4")
	assertErrorCode(t, err, "invalid_request")
	if param := apierror.Body(err).Param; param == nil || *param != "scope" {
		t.Fatalf("param = %v, want scope", param)
	}
}

func TestChallengeRateLimited(t *testing.T) {
	db := testDB(t)
	policies := DefaultPolicies()
	policies.ChallengeLimit = 1
	service, settingsSvc := newServiceHarness(t, db, policies)
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	first, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("first Challenge: %v", err)
	}
	deleteChallenge(t, db, first.ID)

	_, err = service.Challenge(ctx, "login", "1.2.3.4")
	assertErrorCode(t, err, "rate_limited")
}

func TestSolveRejectsWrongAnswerAndBurnsChallenge(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	x, y := challengeAnswer(t, db, challenge.ID)
	_, err = service.Solve(ctx, challenge.ID, x+100, y)
	assertErrorCode(t, err, "captcha_failed")

	_, err = service.Solve(ctx, challenge.ID, x, y)
	assertErrorCode(t, err, "captcha_failed")
}

func TestSolveIsSingleUse(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	x, y := challengeAnswer(t, db, challenge.ID)
	if _, err := service.Solve(ctx, challenge.ID, x, y); err != nil {
		t.Fatalf("first Solve: %v", err)
	}
	_, err = service.Solve(ctx, challenge.ID, x, y)
	assertErrorCode(t, err, "captcha_failed")
}

func TestVerifyTokenConsumesOnce(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	x, y := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, x, y)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}

	if err := service.VerifyToken(ctx, "login", solved.CaptchaToken, "203.0.113.9"); err != nil {
		t.Fatalf("first VerifyToken: %v", err)
	}
	assertErrorCode(t, service.VerifyToken(ctx, "login", solved.CaptchaToken, "203.0.113.9"), "captcha_failed")
}

func TestVerifyTokenRejectsOtherScope(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	x, y := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, x, y)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}

	assertErrorCode(t, service.VerifyToken(ctx, "register", solved.CaptchaToken, "203.0.113.9"), "captcha_failed")
}

func TestVerifyTokenRejectsExpiredToken(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	x, y := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, x, y)
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}

	if _, err := db.Client.CaptchaChallenge.UpdateOneID(challenge.ID).
		SetExpiresAt(time.Now().Add(-time.Minute)).Save(ctx); err != nil {
		t.Fatalf("expire token: %v", err)
	}

	assertErrorCode(t, service.VerifyToken(ctx, "login", solved.CaptchaToken, "203.0.113.9"), "captcha_failed")
}

func TestVerifyTokenRateLimitsFailuresPerIP(t *testing.T) {
	db := testDB(t)
	policies := DefaultPolicies()
	policies.ChallengeLimit = 100
	policies.Failure = ratelimit.FailurePolicy{
		Limit:    1,
		Window:   time.Minute,
		Cooldown: time.Minute,
	}
	service, settingsSvc := newServiceHarness(t, db, policies)
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	assertErrorCode(t, service.VerifyToken(ctx, "login", "", "203.0.113.9"), "captcha_failed")
	assertErrorCode(t, service.VerifyToken(ctx, "login", "not-a-token", "203.0.113.9"), "rate_limited")
	assertErrorCode(t, service.VerifyToken(ctx, "login", "", "198.51.100.7"), "captcha_failed")
}
