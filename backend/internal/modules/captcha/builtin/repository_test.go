package builtin

import (
	"context"
	"errors"
	"testing"
	"time"

	"website/backend/internal/database/ent/captchachallenge"
)

func TestChallengeRepositoryCreateAndFindPending(t *testing.T) {
	db := testDB(t)
	repo := newRepository(db)
	ctx := context.Background()

	now := time.Now()
	id, err := repo.create(ctx, "login", 10, 20, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(id).Exec(context.Background()) })

	rec, err := repo.findPending(ctx, id, now)
	if err != nil {
		t.Fatalf("findPending: %v", err)
	}
	if rec.AnswerX != 10 || rec.AnswerY != 20 {
		t.Fatalf("answer = (%d,%d), want (10,20)", rec.AnswerX, rec.AnswerY)
	}

	if _, err := repo.findPending(ctx, id, now.Add(4*time.Minute)); !errors.Is(err, errChallengeNotFound) {
		t.Fatalf("expired findPending err = %v, want errChallengeNotFound", err)
	}
}

func TestChallengeRepositoryMarkSolvedIsSingleUse(t *testing.T) {
	db := testDB(t)
	repo := newRepository(db)
	ctx := context.Background()

	now := time.Now()
	id, err := repo.create(ctx, "login", 1, 2, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(id).Exec(context.Background()) })

	n, err := repo.markSolved(ctx, id, "hash-"+uniqueTag(t), now, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("markSolved: %v", err)
	}
	if n != 1 {
		t.Fatalf("first markSolved = %d, want 1", n)
	}

	n, err = repo.markSolved(ctx, id, "hash2-"+uniqueTag(t), now, now.Add(5*time.Minute))
	if err != nil {
		t.Fatalf("markSolved again: %v", err)
	}
	if n != 0 {
		t.Fatalf("second markSolved = %d, want 0", n)
	}

	failedID, err := repo.create(ctx, "login", 3, 4, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create failed row: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(failedID).Exec(context.Background()) })

	if n, err := repo.markFailed(ctx, failedID, now); err != nil || n != 1 {
		t.Fatalf("markFailed = (%d,%v), want (1,nil)", n, err)
	}
	if n, err := repo.markSolved(ctx, failedID, "hash3-"+uniqueTag(t), now, now.Add(5*time.Minute)); err != nil || n != 0 {
		t.Fatalf("markSolved after markFailed = (%d,%v), want (0,nil)", n, err)
	}
}

func TestChallengeRepositoryConsumeOnceAndScopeBound(t *testing.T) {
	db := testDB(t)
	repo := newRepository(db)
	ctx := context.Background()

	now := time.Now()
	id, err := repo.create(ctx, "login", 1, 2, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(id).Exec(context.Background()) })

	hash := "consume-" + uniqueTag(t)
	if n, err := repo.markSolved(ctx, id, hash, now, now.Add(5*time.Minute)); err != nil || n != 1 {
		t.Fatalf("markSolved = (%d,%v), want (1,nil)", n, err)
	}

	if ok, err := repo.consume(ctx, "register", hash, now); err != nil || ok {
		t.Fatalf("consume wrong scope = (%v,%v), want (false,nil)", ok, err)
	}
	if ok, err := repo.consume(ctx, "login", hash, now); err != nil || !ok {
		t.Fatalf("consume first = (%v,%v), want (true,nil)", ok, err)
	}
	if ok, err := repo.consume(ctx, "login", hash, now); err != nil || ok {
		t.Fatalf("consume second = (%v,%v), want (false,nil)", ok, err)
	}

	expiredID, err := repo.create(ctx, "login", 5, 6, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("create expired row: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(expiredID).Exec(context.Background()) })

	expiredHash := "expired-" + uniqueTag(t)
	if n, err := repo.markSolved(ctx, expiredID, expiredHash, now, now.Add(5*time.Minute)); err != nil || n != 1 {
		t.Fatalf("markSolved expired row = (%d,%v), want (1,nil)", n, err)
	}
	if _, err := db.Client.CaptchaChallenge.UpdateOneID(expiredID).
		SetExpiresAt(now.Add(-time.Minute)).Save(ctx); err != nil {
		t.Fatalf("expire row: %v", err)
	}
	if ok, err := repo.consume(ctx, "login", expiredHash, now); err != nil || ok {
		t.Fatalf("consume expired = (%v,%v), want (false,nil)", ok, err)
	}
}

func TestChallengeRepositoryDeleteExpired(t *testing.T) {
	db := testDB(t)
	repo := newRepository(db)
	ctx := context.Background()

	now := time.Now()
	expiredID, err := repo.create(ctx, "login", 1, 2, now.Add(-time.Minute))
	if err != nil {
		t.Fatalf("create expired: %v", err)
	}
	liveID, err := repo.create(ctx, "login", 3, 4, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("create live: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.CaptchaChallenge.DeleteOneID(liveID).Exec(context.Background()) })

	if err := repo.deleteExpired(ctx, now); err != nil {
		t.Fatalf("deleteExpired: %v", err)
	}

	if n, err := db.Client.CaptchaChallenge.Query().Where(captchachallenge.IDEQ(expiredID)).Count(ctx); err != nil || n != 0 {
		t.Fatalf("expired row count = (%d,%v), want (0,nil)", n, err)
	}
	if n, err := db.Client.CaptchaChallenge.Query().Where(captchachallenge.IDEQ(liveID)).Count(ctx); err != nil || n != 1 {
		t.Fatalf("live row count = (%d,%v), want (1,nil)", n, err)
	}
}
