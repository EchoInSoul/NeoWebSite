package captcha

import (
	"context"
	"testing"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/modules/captcha/builtin"
	"website/backend/internal/modules/captcha/turnstile"
	"website/backend/internal/modules/settings"
)

func dispatchSettings(t *testing.T, db *database.Database) *settings.Service {
	t.Helper()

	module, err := settings.New(db)
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	return module.Service
}

func TestDispatchChallengeUnavailableWhenBuiltinNotWired(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")
	service := NewService(settingsSvc, nil)

	_, err := service.Challenge(context.Background(), "login", "1.2.3.4")
	assertErrorCode(t, err, "service_unavailable")
}

func TestDispatchVerifyUnavailableWhenBuiltinNotWired(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")
	service := NewService(settingsSvc, nil)

	err := service.Verify(context.Background(), "login", "x", "")
	assertErrorCode(t, err, "service_unavailable")
}

func TestDispatchVerifyPassesWhenCaptchaDisabled(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, false, "builtin", "login,register")
	service := NewService(settingsSvc, nil)

	if err := service.Verify(context.Background(), "login", "", ""); err != nil {
		t.Fatalf("Verify = %v, want nil", err)
	}
}

func TestDispatchVerifyPassesWhenProviderUnknown(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	service := NewService(settingsSvc, nil)
	ctx := context.Background()

	for _, provider := range []string{"none", "geetest"} {
		setCaptchaSettings(t, settingsSvc, true, provider, "login,register")
		if err := service.Verify(ctx, "login", "", ""); err != nil {
			t.Fatalf("provider %q Verify = %v, want nil", provider, err)
		}
	}
}

func TestDispatchVerifyRoutesTurnstileToVerifier(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "turnstile", "login,register")

	ctx := context.Background()
	if err := settingsSvc.SetString(ctx, "captcha", "turnstile_secret_key", "secret"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	t.Cleanup(func() {
		_ = settingsSvc.SetString(context.Background(), "captcha", "turnstile_secret_key", "")
	})

	verifier := &fakeVerifier{outcome: turnstile.Passed}
	service := NewService(settingsSvc, verifier)

	if err := service.Verify(ctx, "login", "token", "1.2.3.4"); err != nil {
		t.Fatalf("Verify passed = %v, want nil", err)
	}
	if verifier.calls != 1 || verifier.token != "token" || verifier.secret != "secret" {
		t.Fatalf("verifier call = %+v", verifier)
	}

	verifier.outcome = turnstile.Rejected
	assertErrorCode(t, service.Verify(ctx, "login", "token", "1.2.3.4"), "captcha_failed")
}

func TestDispatchVerifyRoutesBuiltin(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin", "login,register")

	builtinService, err := builtin.NewWithPolicies(db, settingsSvc, ratelimit.New(), builtin.DefaultPolicies())
	if err != nil {
		t.Fatalf("builtin.NewWithPolicies: %v", err)
	}
	service := newService(settingsSvc, nil, builtinService)

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

	if err := service.Verify(ctx, "login", solved.CaptchaToken, ""); err != nil {
		t.Fatalf("Verify = %v, want nil", err)
	}
	assertErrorCode(t, service.Verify(ctx, "login", solved.CaptchaToken, ""), "captcha_failed")
}
