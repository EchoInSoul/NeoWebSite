package builtin

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	"website/backend/internal/modules/settings"
)

func TestChallengeAndSolveFlow(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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

	answer := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, []Point{{X: answer[0].X, Y: answer[0].Y}})
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
			setCaptchaSettings(t, settingsSvc, tc.enabled, tc.provider, modeSlide)
			_, err := service.Challenge(ctx, "login", "1.2.3.4")
			assertErrorCode(t, err, "captcha_disabled")
		})
	}
}

func TestChallengeRejectsUnknownScope(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	answer := challengeAnswer(t, db, challenge.ID)
	_, err = service.Solve(ctx, challenge.ID, []Point{{X: answer[0].X + 100, Y: answer[0].Y}})
	assertErrorCode(t, err, "captcha_failed")

	_, err = service.Solve(ctx, challenge.ID, []Point{{X: answer[0].X, Y: answer[0].Y}})
	assertErrorCode(t, err, "captcha_failed")
}

func TestSolveIsSingleUse(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	answer := challengeAnswer(t, db, challenge.ID)
	click := []Point{{X: answer[0].X, Y: answer[0].Y}}
	if _, err := service.Solve(ctx, challenge.ID, click); err != nil {
		t.Fatalf("first Solve: %v", err)
	}
	_, err = service.Solve(ctx, challenge.ID, click)
	assertErrorCode(t, err, "captcha_failed")
}

func TestVerifyTokenConsumesOnce(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	answer := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, []Point{{X: answer[0].X, Y: answer[0].Y}})
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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	answer := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, []Point{{X: answer[0].X, Y: answer[0].Y}})
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}

	assertErrorCode(t, service.VerifyToken(ctx, "register", solved.CaptchaToken, "203.0.113.9"), "captcha_failed")
}

func TestVerifyTokenRejectsExpiredToken(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	ctx := context.Background()
	challenge, err := service.Challenge(ctx, "login", "1.2.3.4")
	if err != nil {
		t.Fatalf("Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	answer := challengeAnswer(t, db, challenge.ID)
	solved, err := service.Solve(ctx, challenge.ID, []Point{{X: answer[0].X, Y: answer[0].Y}})
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
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	ctx := context.Background()
	assertErrorCode(t, service.VerifyToken(ctx, "login", "", "203.0.113.9"), "captcha_failed")
	assertErrorCode(t, service.VerifyToken(ctx, "login", "not-a-token", "203.0.113.9"), "rate_limited")
	assertErrorCode(t, service.VerifyToken(ctx, "login", "", "198.51.100.7"), "captcha_failed")
}

func clickChallenge(t *testing.T, service *Service, settingsSvc *settings.Service, db *database.Database, scope, ip string) ChallengeResponse {
	t.Helper()

	setCaptchaSettings(t, settingsSvc, true, "builtin", modeClick)
	challenge, err := service.Challenge(context.Background(), scope, ip)
	if err != nil {
		t.Fatalf("click Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)
	return challenge
}

func TestClickChallengeShape(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.4")

	if challenge.PromptImage == "" || !strings.HasPrefix(challenge.PromptImage, "data:image/png;base64,") {
		t.Fatalf("prompt image = %.40q", challenge.PromptImage)
	}
	if challenge.PromptText == "" {
		t.Fatal("prompt text is empty")
	}
	if challenge.ExpectedClicks != 3 {
		t.Fatalf("expected clicks = %d, want 3", challenge.ExpectedClicks)
	}
	if challenge.TileImage != "" {
		t.Fatalf("tile image = %.40q, want empty for click mode", challenge.TileImage)
	}

	dots := challengeAnswer(t, db, challenge.ID)
	if len(dots) != 3 {
		t.Fatalf("stored dots = %d, want 3", len(dots))
	}
	if got := challengePromptText(t, db, challenge.ID); got != challenge.PromptText {
		t.Fatalf("stored prompt text = %q, want %q", got, challenge.PromptText)
	}
}

func TestSlideChallengeShapeUnchanged(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)

	challenge, err := service.Challenge(context.Background(), "login", "1.2.3.5")
	if err != nil {
		t.Fatalf("slide Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)

	if !strings.HasPrefix(challenge.TileImage, "data:image/png;base64,") {
		t.Fatalf("tile image = %.40q", challenge.TileImage)
	}
	if challenge.PromptImage != "" || challenge.PromptText != "" || challenge.ExpectedClicks != 0 {
		t.Fatalf("click fields leaked into slide response: %+v", challenge)
	}

	dots := challengeAnswer(t, db, challenge.ID)
	if len(dots) != 1 {
		t.Fatalf("stored dots = %d, want 1", len(dots))
	}
	if got := challengePromptText(t, db, challenge.ID); got != "" {
		t.Fatalf("stored prompt text = %q, want empty", got)
	}
}

func TestClickSolveInOrder(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.6")
	dots := challengeAnswer(t, db, challenge.ID)

	solved, err := service.Solve(ctx, challenge.ID, []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}})
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

func TestClickSolveAcceptsClicksInsideGlyphBox(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.14")
	dots := challengeAnswer(t, db, challenge.ID)

	centers := make([]Point, 0, len(dots))
	for _, dot := range dots {
		centers = append(centers, Point{X: dot.X + dot.Width/2, Y: dot.Y + dot.Height/2})
	}
	if _, err := service.Solve(ctx, challenge.ID, centers); err != nil {
		t.Fatalf("Solve at glyph centers: %v", err)
	}
}

func TestClickSolveRejectsShuffledOrder(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.7")
	dots := challengeAnswer(t, db, challenge.ID)

	rotated := []Point{{X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}, {X: dots[0].X, Y: dots[0].Y}}
	_, err := service.Solve(ctx, challenge.ID, rotated)
	assertErrorCode(t, err, "captcha_failed")

	ordered := []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}}
	_, err = service.Solve(ctx, challenge.ID, ordered)
	assertErrorCode(t, err, "captcha_failed")
}

func TestClickSolveRejectsSetMatch(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.15")
	dots := challengeAnswer(t, db, challenge.ID)

	swapped := []Point{{X: dots[1].X, Y: dots[1].Y}, {X: dots[0].X, Y: dots[0].Y}, {X: dots[2].X, Y: dots[2].Y}}
	_, err := service.Solve(ctx, challenge.ID, swapped)
	assertErrorCode(t, err, "captcha_failed")

	other := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.16")
	otherDots := challengeAnswer(t, db, other.ID)
	ordered := []Point{{X: otherDots[0].X, Y: otherDots[0].Y}, {X: otherDots[1].X, Y: otherDots[1].Y}, {X: otherDots[2].X, Y: otherDots[2].Y}}
	if _, err := service.Solve(ctx, other.ID, ordered); err != nil {
		t.Fatalf("Solve on a fresh challenge = %v, want nil", err)
	}
}

func TestClickSolveRejectsOneWrong(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.8")
	dots := challengeAnswer(t, db, challenge.ID)

	wrong := []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X + 90, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}}
	_, err := service.Solve(ctx, challenge.ID, wrong)
	assertErrorCode(t, err, "captcha_failed")

	ordered := []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}}
	_, err = service.Solve(ctx, challenge.ID, ordered)
	assertErrorCode(t, err, "captcha_failed")
}

func TestClickSolveIsSingleUse(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.9")
	dots := challengeAnswer(t, db, challenge.ID)
	clicks := []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}}

	if _, err := service.Solve(ctx, challenge.ID, clicks); err != nil {
		t.Fatalf("first Solve: %v", err)
	}
	_, err := service.Solve(ctx, challenge.ID, clicks)
	assertErrorCode(t, err, "captcha_failed")
}

func TestClickSolveRejectsExpiredChallenge(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.10")
	dots := challengeAnswer(t, db, challenge.ID)
	if _, err := db.Client.CaptchaChallenge.UpdateOneID(challenge.ID).
		SetExpiresAt(time.Now().Add(-time.Minute)).Save(ctx); err != nil {
		t.Fatalf("expire challenge: %v", err)
	}

	clicks := []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}}
	_, err := service.Solve(ctx, challenge.ID, clicks)
	assertErrorCode(t, err, "captcha_failed")
}

func TestClickTokenIsScopeBound(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.11")
	dots := challengeAnswer(t, db, challenge.ID)

	solved, err := service.Solve(ctx, challenge.ID, []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}})
	if err != nil {
		t.Fatalf("Solve: %v", err)
	}

	assertErrorCode(t, service.VerifyToken(ctx, "register", solved.CaptchaToken, "1.2.3.11"), "captcha_failed")
	if err := service.VerifyToken(ctx, "login", solved.CaptchaToken, "1.2.3.11"); err != nil {
		t.Fatalf("VerifyToken correct scope = %v, want nil", err)
	}
	assertErrorCode(t, service.VerifyToken(ctx, "login", solved.CaptchaToken, "1.2.3.11"), "captcha_failed")
}

func TestClickSolveRejectsWrongClickCount(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	challenge := clickChallenge(t, service, settingsSvc, db, "login", "1.2.3.12")
	dots := challengeAnswer(t, db, challenge.ID)

	_, err := service.Solve(ctx, challenge.ID, []Point{{X: dots[0].X, Y: dots[0].Y}})
	assertErrorCode(t, err, "invalid_request")

	_, err = service.Solve(ctx, challenge.ID, []Point{
		{X: dots[0].X, Y: dots[0].Y},
		{X: dots[1].X, Y: dots[1].Y},
		{X: dots[2].X, Y: dots[2].Y},
		{X: dots[0].X, Y: dots[0].Y},
	})
	assertErrorCode(t, err, "invalid_request")

	ordered := []Point{{X: dots[0].X, Y: dots[0].Y}, {X: dots[1].X, Y: dots[1].Y}, {X: dots[2].X, Y: dots[2].Y}}
	if _, err := service.Solve(ctx, challenge.ID, ordered); err != nil {
		t.Fatalf("Solve after wrong counts = %v, want nil (count mismatch must not burn the challenge)", err)
	}
}

func TestSlideSolveRejectsClickShape(t *testing.T) {
	db := testDB(t)
	service, settingsSvc := newServiceHarness(t, db, DefaultPolicies())
	ctx := context.Background()

	setCaptchaSettings(t, settingsSvc, true, "builtin", modeSlide)
	challenge, err := service.Challenge(ctx, "login", "1.2.3.13")
	if err != nil {
		t.Fatalf("slide Challenge: %v", err)
	}
	deleteChallenge(t, db, challenge.ID)
	dots := challengeAnswer(t, db, challenge.ID)

	_, err = service.Solve(ctx, challenge.ID, []Point{
		{X: dots[0].X, Y: dots[0].Y},
		{X: dots[0].X, Y: dots[0].Y},
		{X: dots[0].X, Y: dots[0].Y},
	})
	assertErrorCode(t, err, "invalid_request")
}
