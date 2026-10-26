package passwordreset

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/database"
	pwreset "website/backend/internal/database/ent/passwordreset"
	"website/backend/internal/database/ent/user"
)

func resetRowCount(t *testing.T, db *database.Database, userID uuid.UUID) int {
	t.Helper()

	count, err := db.Client.PasswordReset.Query().Where(pwreset.UserIDEQ(userID)).Count(context.Background())
	if err != nil {
		t.Fatalf("count resets: %v", err)
	}
	return count
}

func TestReplaceResetUpsertsSingleRow(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	created := createTestUser(t, db, tag)
	repo := newRepository(db)

	firstExpiry := time.Now().Add(10 * time.Minute)
	if _, err := repo.replaceReset(ctx, created.ID, "hash-a-"+tag, firstExpiry); err != nil {
		t.Fatalf("first replaceReset: %v", err)
	}

	secondExpiry := time.Now().Add(40 * time.Minute)
	if _, err := repo.replaceReset(ctx, created.ID, "hash-b-"+tag, secondExpiry); err != nil {
		t.Fatalf("second replaceReset: %v", err)
	}

	if count := resetRowCount(t, db, created.ID); count != 1 {
		t.Fatalf("reset rows = %d, want 1", count)
	}
	found, err := repo.findReset(ctx, "hash-b-"+tag)
	if err != nil {
		t.Fatalf("findReset(new hash): %v", err)
	}
	if !found.ExpiresAt.Round(time.Minute).Equal(secondExpiry.Round(time.Minute)) {
		t.Fatalf("expires at = %v, want %v", found.ExpiresAt, secondExpiry)
	}
	if _, err := repo.findReset(ctx, "hash-a-"+tag); !errors.Is(err, errResetNotFound) {
		t.Fatalf("findReset(stale hash) error = %v, want errResetNotFound", err)
	}
}

func TestReplaceResetClearsExpiredRows(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	expired := time.Now().Add(-time.Hour)

	staleA := createTestUser(t, db, "a"+tag)
	staleB := createTestUser(t, db, "b"+tag)
	fresh := createTestUser(t, db, "c"+tag)

	for _, pair := range []struct {
		id   uuid.UUID
		hash string
	}{{staleA.ID, "stale-a-" + tag}, {staleB.ID, "stale-b-" + tag}} {
		if _, err := db.Client.PasswordReset.Create().
			SetUserID(pair.id).
			SetTokenHash(pair.hash).
			SetExpiresAt(expired).
			Save(ctx); err != nil {
			t.Fatalf("seed expired reset: %v", err)
		}
	}

	repo := newRepository(db)
	if _, err := repo.replaceReset(ctx, fresh.ID, "fresh-"+tag, time.Now().Add(resetTTL)); err != nil {
		t.Fatalf("replaceReset: %v", err)
	}

	if count := resetRowCount(t, db, staleA.ID); count != 0 {
		t.Fatalf("stale A rows = %d, want 0", count)
	}
	if count := resetRowCount(t, db, staleB.ID); count != 0 {
		t.Fatalf("stale B rows = %d, want 0", count)
	}
	if count := resetRowCount(t, db, fresh.ID); count != 1 {
		t.Fatalf("fresh rows = %d, want 1", count)
	}
}

func TestFindResetMissing(t *testing.T) {
	db := testDB(t)
	repo := newRepository(db)

	if _, err := repo.findReset(context.Background(), "absent"); !errors.Is(err, errResetNotFound) {
		t.Fatalf("findReset error = %v, want errResetNotFound", err)
	}
}

func TestDeleteResetRequiresTokenHash(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	created := createTestUser(t, db, tag)
	repo := newRepository(db)

	if _, err := repo.replaceReset(ctx, created.ID, "hash-a-"+tag, time.Now().Add(resetTTL)); err != nil {
		t.Fatalf("first replaceReset: %v", err)
	}
	id, err := repo.replaceReset(ctx, created.ID, "hash-b-"+tag, time.Now().Add(resetTTL))
	if err != nil {
		t.Fatalf("second replaceReset: %v", err)
	}

	deleted, err := repo.deleteReset(ctx, id, "hash-a-"+tag)
	if err != nil {
		t.Fatalf("deleteReset with stale hash: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted with stale hash = %d, want 0", deleted)
	}
	if count := resetRowCount(t, db, created.ID); count != 1 {
		t.Fatalf("reset rows after stale-hash delete = %d, want 1", count)
	}

	deleted, err = repo.deleteReset(ctx, id, "hash-b-"+tag)
	if err != nil {
		t.Fatalf("deleteReset with current hash: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted with current hash = %d, want 1", deleted)
	}
	if count := resetRowCount(t, db, created.ID); count != 0 {
		t.Fatalf("reset rows after current-hash delete = %d, want 0", count)
	}
}

func TestCompleteResetConsumesOnce(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	created := createTestUserWithPassword(t, db, tag, "Initial123")
	repo := newRepository(db)

	hash := "consume-" + tag
	id, err := repo.replaceReset(ctx, created.ID, hash, time.Now().Add(resetTTL))
	if err != nil {
		t.Fatalf("replaceReset: %v", err)
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte("NewPass123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	if err := repo.completeReset(ctx, id, hash, created.ID, string(newHash)); err != nil {
		t.Fatalf("first completeReset: %v", err)
	}
	if err := repo.completeReset(ctx, id, hash, created.ID, string(newHash)); !errors.Is(err, errResetNotFound) {
		t.Fatalf("second completeReset error = %v, want errResetNotFound", err)
	}

	stored, err := db.Client.User.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("NewPass123")) != nil {
		t.Fatal("password hash did not change to the new password")
	}
	if stored.AuthVersion != created.AuthVersion+1 {
		t.Fatalf("auth version = %d, want %d", stored.AuthVersion, created.AuthVersion+1)
	}
	if count := resetRowCount(t, db, created.ID); count != 0 {
		t.Fatalf("reset rows after completion = %d, want 0", count)
	}
}

func TestCompleteResetStaleHash(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	created := createTestUserWithPassword(t, db, tag, "Initial123")
	repo := newRepository(db)

	hash := "stale-" + tag
	id, err := repo.replaceReset(ctx, created.ID, hash, time.Now().Add(resetTTL))
	if err != nil {
		t.Fatalf("replaceReset: %v", err)
	}
	if _, err := db.Client.PasswordReset.UpdateOneID(id).SetTokenHash("replaced-" + tag).Save(ctx); err != nil {
		t.Fatalf("rotate token hash: %v", err)
	}

	before, err := db.Client.User.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if err := repo.completeReset(ctx, id, hash, created.ID, "whatever"); !errors.Is(err, errResetNotFound) {
		t.Fatalf("completeReset with stale hash error = %v, want errResetNotFound", err)
	}

	after, err := db.Client.User.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read user again: %v", err)
	}
	if after.PasswordHash != before.PasswordHash {
		t.Fatal("password hash changed on a stale-hash completion")
	}
	if after.AuthVersion != before.AuthVersion {
		t.Fatalf("auth version = %d, want %d", after.AuthVersion, before.AuthVersion)
	}
}

func TestCompleteResetBumpsAuthVersion(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	tag := uniqueTag(t)
	created := createTestUserWithPassword(t, db, tag, "Initial123")
	repo := newRepository(db)

	if _, err := db.Client.User.UpdateOneID(created.ID).AddAuthVersion(4).Save(ctx); err != nil {
		t.Fatalf("seed auth version: %v", err)
	}
	start, err := db.Client.User.Query().Where(user.IDEQ(created.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("read seeded user: %v", err)
	}

	hash := "version-" + tag
	id, err := repo.replaceReset(ctx, created.ID, hash, time.Now().Add(resetTTL))
	if err != nil {
		t.Fatalf("replaceReset: %v", err)
	}
	if err := repo.completeReset(ctx, id, hash, created.ID, "new-hash"); err != nil {
		t.Fatalf("completeReset: %v", err)
	}

	done, err := db.Client.User.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("read user: %v", err)
	}
	if done.AuthVersion != start.AuthVersion+1 {
		t.Fatalf("auth version = %d, want %d", done.AuthVersion, start.AuthVersion+1)
	}
	if done.PasswordHash != "new-hash" {
		t.Fatalf("password hash = %q, want the value written by completeReset", done.PasswordHash)
	}
}
