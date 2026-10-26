package builtin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/captchachallenge"
)

var errChallengeNotFound = errors.New("captcha: challenge not found")

type challengeRecord struct {
	ID         uuid.UUID
	Dots       []AnswerDot
	PromptText string
}

type repository struct {
	client *ent.Client
}

func newRepository(db *database.Database) *repository {
	return &repository{client: db.Client}
}

func (r *repository) create(ctx context.Context, scope, promptText string, dots []AnswerDot, expiresAt time.Time) (uuid.UUID, error) {
	encoded, err := json.Marshal(AnswerDots{Dots: dots})
	if err != nil {
		return uuid.Nil, err
	}

	created, err := r.client.CaptchaChallenge.Create().
		SetScope(scope).
		SetAnswerDots(encoded).
		SetPromptText(promptText).
		SetStatus(statusPending).
		SetExpiresAt(expiresAt).
		Save(ctx)
	if err != nil {
		return uuid.Nil, err
	}
	return created.ID, nil
}

func (r *repository) findPending(ctx context.Context, id uuid.UUID, now time.Time) (challengeRecord, error) {
	found, err := r.client.CaptchaChallenge.Query().
		Where(
			captchachallenge.IDEQ(id),
			captchachallenge.StatusEQ(statusPending),
			captchachallenge.ExpiresAtGT(now),
		).
		Only(ctx)
	if ent.IsNotFound(err) {
		return challengeRecord{}, errChallengeNotFound
	}
	if err != nil {
		return challengeRecord{}, err
	}
	dots, err := decodeAnswerDots(found.AnswerDots)
	if err != nil {
		return challengeRecord{}, err
	}
	return challengeRecord{
		ID:         found.ID,
		Dots:       dots,
		PromptText: found.PromptText,
	}, nil
}

func (r *repository) markSolved(ctx context.Context, id uuid.UUID, tokenHash string, now, expiresAt time.Time) (int, error) {
	return r.client.CaptchaChallenge.Update().
		Where(
			captchachallenge.IDEQ(id),
			captchachallenge.StatusEQ(statusPending),
			captchachallenge.ExpiresAtGT(now),
		).
		SetStatus(statusSolved).
		SetTokenHash(tokenHash).
		SetExpiresAt(expiresAt).
		Save(ctx)
}

func (r *repository) markFailed(ctx context.Context, id uuid.UUID, now time.Time) (int, error) {
	return r.client.CaptchaChallenge.Update().
		Where(
			captchachallenge.IDEQ(id),
			captchachallenge.StatusEQ(statusPending),
			captchachallenge.ExpiresAtGT(now),
		).
		SetStatus(statusFailed).
		Save(ctx)
}

func (r *repository) consume(ctx context.Context, scope, tokenHash string, now time.Time) (bool, error) {
	n, err := r.client.CaptchaChallenge.Update().
		Where(
			captchachallenge.TokenHashEQ(tokenHash),
			captchachallenge.ScopeEQ(scope),
			captchachallenge.StatusEQ(statusSolved),
			captchachallenge.ExpiresAtGT(now),
		).
		SetStatus(statusConsumed).
		Save(ctx)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (r *repository) deleteExpired(ctx context.Context, now time.Time) error {
	_, err := r.client.CaptchaChallenge.Delete().
		Where(captchachallenge.ExpiresAtLT(now)).
		Exec(ctx)
	return err
}

func decodeAnswerDots(raw json.RawMessage) ([]AnswerDot, error) {
	var payload AnswerDots
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("captcha: decode answer dots: %w", err)
	}
	return payload.Dots, nil
}
