package builtin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/wenlng/go-captcha/v2/click"
	"github.com/wenlng/go-captcha/v2/slide"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/ratelimit"
	"website/backend/internal/database"
	captchapolicy "website/backend/internal/modules/captcha/policy"
	"website/backend/internal/modules/settings"
)

const (
	providerBuiltin = "builtin"

	modeSlide = "slide"
	modeClick = "click"

	clickDots = 3
)

type Settings interface {
	GetBool(ctx context.Context, group, name string) (bool, error)
	GetString(ctx context.Context, group, name string) (string, error)
}

type Policies struct {
	ChallengeLimit  int
	ChallengeWindow time.Duration
	ChallengeTTL    time.Duration
	TokenTTL        time.Duration
	AnswerPadding   int
	Failure         ratelimit.FailurePolicy
}

func DefaultPolicies() Policies {
	return Policies{
		ChallengeLimit:  5,
		ChallengeWindow: 10 * time.Minute,
		ChallengeTTL:    3 * time.Minute,
		TokenTTL:        5 * time.Minute,
		AnswerPadding:   4,
		Failure:         captchapolicy.Failure(),
	}
}

func (p Policies) validate(dots []AnswerDot, clicks []Point) bool {
	if len(dots) != len(clicks) || len(dots) == 0 {
		return false
	}

	if len(dots) == 1 {
		return slide.Validate(clicks[0].X, clicks[0].Y, dots[0].X, dots[0].Y, p.AnswerPadding)
	}

	for i, dot := range dots {
		if !click.Validate(clicks[i].X, clicks[i].Y, dot.X, dot.Y, dot.Width, dot.Height, p.AnswerPadding) {
			return false
		}
	}
	return true
}

type Service struct {
	settings  Settings
	repo      *repository
	generator *generator
	clickGen  *clickGenerator
	limiter   *ratelimit.Limiter
	policies  Policies
}

func New(db *database.Database, settingsService Settings, limiter *ratelimit.Limiter) (*Service, error) {
	return NewWithPolicies(db, settingsService, limiter, DefaultPolicies())
}

func NewWithPolicies(db *database.Database, settingsService Settings, limiter *ratelimit.Limiter, policies Policies) (*Service, error) {
	generator, err := newGenerator()
	if err != nil {
		return nil, err
	}
	clickGen, err := newClickGenerator()
	if err != nil {
		return nil, err
	}
	return newService(settingsService, newRepository(db), generator, clickGen, limiter, policies), nil
}

func newService(settingsService Settings, repo *repository, generator *generator, clickGen *clickGenerator, limiter *ratelimit.Limiter, policies Policies) *Service {
	return &Service{
		settings:  settingsService,
		repo:      repo,
		generator: generator,
		clickGen:  clickGen,
		limiter:   limiter,
		policies:  policies,
	}
}

func (s *Service) Challenge(ctx context.Context, scope, ip string) (ChallengeResponse, error) {
	if scope != settings.CaptchaScopeLogin && scope != settings.CaptchaScopeRegister {
		return ChallengeResponse{}, apierror.WithParam(apierror.ErrInvalidRequest, "scope")
	}

	allowed, retryAfter := ratelimit.AllowWindow(
		s.limiter,
		ratelimit.WindowPolicy{Limit: s.policies.ChallengeLimit, Window: s.policies.ChallengeWindow},
		ratelimit.IPKey("captcha-challenges", ip),
	)
	if !allowed {
		return ChallengeResponse{}, apierror.RateLimited(retryAfter)
	}

	enabled, err := s.settings.GetBool(ctx, "captcha", "captcha_enabled")
	if err != nil {
		return ChallengeResponse{}, err
	}
	provider, err := s.settings.GetString(ctx, "captcha", "captcha_provider")
	if err != nil {
		return ChallengeResponse{}, err
	}
	if !enabled || provider != providerBuiltin {
		return ChallengeResponse{}, apierror.ErrCaptchaDisabled
	}

	mode, err := s.settings.GetString(ctx, "captcha", "captcha_mode")
	if err != nil {
		return ChallengeResponse{}, fmt.Errorf("captcha: read captcha_mode: %w (%w)", err, apierror.ErrServiceUnavailable)
	}

	now := time.Now()
	if err := s.repo.deleteExpired(ctx, now); err != nil {
		return ChallengeResponse{}, err
	}

	expiresAt := now.Add(s.policies.ChallengeTTL)

	if mode == modeClick {
		return s.clickChallenge(ctx, scope, now, expiresAt)
	}
	if mode != modeSlide {
		return ChallengeResponse{}, apierror.ErrInternal
	}

	return s.slideChallenge(ctx, scope, now, expiresAt)
}

func (s *Service) slideChallenge(ctx context.Context, scope string, now, expiresAt time.Time) (ChallengeResponse, error) {
	generated, err := s.generator.generate()
	if err != nil {
		return ChallengeResponse{}, err
	}

	id, err := s.repo.create(ctx, scope, "", []AnswerDot{{
		X:      generated.AnswerX,
		Y:      generated.AnswerY,
		Width:  generated.TileWidth,
		Height: generated.TileHeight,
	}}, expiresAt)
	if err != nil {
		return ChallengeResponse{}, err
	}

	return ChallengeResponse{
		ID:           id,
		MasterImage:  generated.MasterImage,
		TileImage:    generated.TileImage,
		MasterWidth:  generated.MasterWidth,
		MasterHeight: generated.MasterHeight,
		TileWidth:    generated.TileWidth,
		TileHeight:   generated.TileHeight,
		TileX:        generated.TileX,
		TileY:        generated.TileY,
		ExpiresAt:    expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) clickChallenge(ctx context.Context, scope string, now, expiresAt time.Time) (ChallengeResponse, error) {
	generated, err := s.clickGen.generate()
	if err != nil {
		return ChallengeResponse{}, err
	}

	id, err := s.repo.create(ctx, scope, generated.PromptText, generated.Dots, expiresAt)
	if err != nil {
		return ChallengeResponse{}, err
	}

	return ChallengeResponse{
		ID:             id,
		MasterImage:    generated.MasterImage,
		PromptImage:    generated.PromptImage,
		PromptText:     generated.PromptText,
		MasterWidth:    generated.MasterWidth,
		MasterHeight:   generated.MasterHeight,
		ExpectedClicks: clickDots,
		ExpiresAt:      expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) Solve(ctx context.Context, id uuid.UUID, clicks []Point) (SolveChallengeResponse, error) {
	now := time.Now()
	record, err := s.repo.findPending(ctx, id, now)
	if errors.Is(err, errChallengeNotFound) {
		return SolveChallengeResponse{}, apierror.ErrCaptchaRejected
	}
	if err != nil {
		return SolveChallengeResponse{}, err
	}

	if len(record.Dots) != len(clicks) {
		return SolveChallengeResponse{}, apierror.WithParam(apierror.ErrInvalidRequest, "clicks")
	}

	if !s.policies.validate(record.Dots, clicks) {
		if _, err := s.repo.markFailed(ctx, id, now); err != nil {
			return SolveChallengeResponse{}, err
		}
		return SolveChallengeResponse{}, apierror.ErrCaptchaRejected
	}

	token, err := newCaptchaToken()
	if err != nil {
		return SolveChallengeResponse{}, err
	}

	expiresAt := now.Add(s.policies.TokenTTL)
	updated, err := s.repo.markSolved(ctx, id, hashToken(token), now, expiresAt)
	if err != nil {
		return SolveChallengeResponse{}, err
	}
	if updated == 0 {
		return SolveChallengeResponse{}, apierror.ErrCaptchaRejected
	}

	return SolveChallengeResponse{
		CaptchaToken: token,
		ExpiresAt:    expiresAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) VerifyToken(ctx context.Context, scope, token, ip string) error {
	blocked, retryAfter := captchapolicy.Allow(s.limiter, s.policies.Failure, ip)
	if blocked {
		return apierror.RateLimited(retryAfter)
	}

	if token == "" {
		captchapolicy.Record(s.limiter, s.policies.Failure, ip)
		return apierror.ErrCaptchaFailed
	}

	consumed, err := s.repo.consume(ctx, scope, hashToken(token), time.Now())
	if err != nil {
		return err
	}
	if !consumed {
		captchapolicy.Record(s.limiter, s.policies.Failure, ip)
		return apierror.ErrCaptchaFailed
	}

	return nil
}

func newCaptchaToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("captcha: generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
