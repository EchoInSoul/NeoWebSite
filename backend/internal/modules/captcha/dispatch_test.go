package captcha

import (
	"context"
	"strings"
	"testing"
	"time"

	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/settingvalue"
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
	setCaptchaSettings(t, settingsSvc, true, "builtin")
	service := NewService(settingsSvc, nil)

	_, err := service.Challenge(context.Background(), "login", "1.2.3.4")
	assertErrorCode(t, err, "service_unavailable")
}

func TestDispatchVerifyUnavailableWhenBuiltinNotWired(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin")
	service := NewService(settingsSvc, nil)

	err := service.Verify(context.Background(), "login", "x", "")
	assertErrorCode(t, err, "service_unavailable")
}

func TestDispatchVerifyPassesWhenCaptchaDisabled(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, false, "builtin")
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
		setCaptchaSettings(t, settingsSvc, true, provider)
		if err := service.Verify(ctx, "login", "", ""); err != nil {
			t.Fatalf("provider %q Verify = %v, want nil", provider, err)
		}
	}
}

func TestDispatchVerifyRoutesTurnstileToVerifier(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "turnstile")

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

func TestChallengeFailsWhenModeRowMissing(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin")
	setCaptchaMode(t, settingsSvc, "slide")

	ctx := context.Background()
	deleted, err := db.Client.SettingValue.Delete().
		Where(settingvalue.SettingGroupEQ("captcha"), settingvalue.NameEQ("captcha_mode")).
		Exec(ctx)
	if err != nil {
		t.Fatalf("delete mode row: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted mode rows = %d, want 1", deleted)
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		updated, err := db.Client.SettingValue.Update().
			Where(settingvalue.SettingGroupEQ("captcha"), settingvalue.NameEQ("captcha_mode")).
			SetValue([]byte(`"slide"`)).
			Save(cleanupCtx)
		if err != nil {
			t.Fatalf("restore mode row: %v", err)
		}
		if updated > 0 {
			return
		}
		if _, err := db.Client.SettingValue.Create().
			SetSettingGroup("captcha").
			SetName("captcha_mode").
			SetValue([]byte(`"slide"`)).
			Save(cleanupCtx); err != nil {
			t.Fatalf("recreate mode row: %v", err)
		}
	})

	builtinService, err := builtin.NewWithPolicies(db, settingsSvc, ratelimit.New(), builtin.DefaultPolicies())
	if err != nil {
		t.Fatalf("builtin.NewWithPolicies: %v", err)
	}
	service := newService(settingsSvc, nil, builtinService)

	_, err = service.Challenge(ctx, "login", "1.2.3.4")
	assertErrorCode(t, err, "service_unavailable")
	if !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v, want the settings root cause retained", err)
	}
}

func TestChallengeFailsWhenModeValueInvalid(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin")

	ctx := context.Background()
	if _, err := db.Client.SettingValue.Update().
		Where(settingvalue.SettingGroupEQ("captcha"), settingvalue.NameEQ("captcha_mode")).
		SetValue([]byte(`"foo"`)).
		Save(ctx); err != nil {
		t.Fatalf("write bad mode row: %v", err)
	}
	t.Cleanup(func() {
		_ = settingsSvc.SetString(context.Background(), "captcha", "captcha_mode", "slide")
	})

	builtinService, err := builtin.NewWithPolicies(db, settingsSvc, ratelimit.New(), builtin.DefaultPolicies())
	if err != nil {
		t.Fatalf("builtin.NewWithPolicies: %v", err)
	}
	service := newService(settingsSvc, nil, builtinService)

	_, err = service.Challenge(ctx, "login", "1.2.3.4")
	assertErrorCode(t, err, "internal_error")
}

func TestChallengeSlideUnchanged(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin")
	setCaptchaMode(t, settingsSvc, "slide")

	builtinService, err := builtin.NewWithPolicies(db, settingsSvc, ratelimit.New(), builtin.DefaultPolicies())
	if err != nil {
		t.Fatalf("builtin.NewWithPolicies: %v", err)
	}
	service := newService(settingsSvc, nil, builtinService)

	challenge, err := service.Challenge(context.Background(), "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	if !strings.HasPrefix(challenge.TileImage, "data:image/png;base64,") {
		t.Fatalf("tile image = %.40q", challenge.TileImage)
	}
	if challenge.PromptImage != "" || challenge.PromptText != "" || challenge.ExpectedClicks != 0 {
		t.Fatalf("click fields leaked into slide response: %+v", challenge)
	}

	dots := challengeDots(t, db, challenge.ID)
	if len(dots) != 1 {
		t.Fatalf("stored dots = %d, want 1", len(dots))
	}
	if got := challengePromptText(t, db, challenge.ID); got != "" {
		t.Fatalf("stored prompt text = %q, want empty", got)
	}
}

func TestDispatchVerifyRoutesBuiltin(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	setCaptchaSettings(t, settingsSvc, true, "builtin")

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

	dots := challengeDots(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, []builtin.Point{{X: dots[0].X, Y: dots[0].Y}})
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}

	if err := service.Verify(ctx, "login", solved.CaptchaToken, ""); err != nil {
		t.Fatalf("Verify = %v, want nil", err)
	}
	assertErrorCode(t, service.Verify(ctx, "login", solved.CaptchaToken, ""), "captcha_failed")
}

func TestCaptchaFailureLimitSharedAcrossProviders(t *testing.T) {
	db := testDB(t)
	settingsSvc := dispatchSettings(t, db)
	limiter := ratelimit.New()
	failure := ratelimit.FailurePolicy{
		Limit:    1,
		Window:   time.Minute,
		Cooldown: time.Minute,
	}

	builtinPolicies := builtin.DefaultPolicies()
	builtinPolicies.ChallengeLimit = 100
	builtinPolicies.Failure = failure
	builtinService, err := builtin.NewWithPolicies(db, settingsSvc, limiter, builtinPolicies)
	if err != nil {
		t.Fatalf("builtin.NewWithPolicies: %v", err)
	}

	verifier := &fakeVerifier{outcome: turnstile.Passed}
	turnstileService := turnstile.NewServiceWithPolicies(
		settingsSvc,
		verifier,
		limiter,
		turnstile.Policies{Failure: failure},
	)
	service := newService(settingsSvc, turnstileService, builtinService)

	ctx := context.Background()
	setCaptchaSettings(t, settingsSvc, true, "builtin")
	assertErrorCode(t, service.Verify(ctx, "login", "", "203.0.113.9"), "captcha_failed")

	setCaptchaSettings(t, settingsSvc, true, "turnstile")
	if err := settingsSvc.SetString(ctx, "captcha", "turnstile_secret_key", "secret"); err != nil {
		t.Fatalf("set secret: %v", err)
	}
	t.Cleanup(func() {
		_ = settingsSvc.SetString(context.Background(), "captcha", "turnstile_secret_key", "")
	})

	assertErrorCode(t, service.Verify(ctx, "login", "token", "203.0.113.9"), "rate_limited")
	if verifier.calls != 0 {
		t.Fatalf("verifier calls while shared bucket limited = %d, want 0", verifier.calls)
	}

	if err := service.Verify(ctx, "login", "token", "198.51.100.7"); err != nil {
		t.Fatalf("other IP Verify = %v, want nil", err)
	}
	if verifier.calls != 1 {
		t.Fatalf("verifier calls after other IP = %d, want 1", verifier.calls)
	}
}
