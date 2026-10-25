package twofactor

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
	"website/backend/internal/database/ent/recoverycode"
)

func testDB(t *testing.T) *database.Database {
	t.Helper()

	url := os.Getenv("DATABASE_URL_TEST")
	if url == "" {
		t.Skip("SKIP: DATABASE_URL_TEST is not set")
	}

	db, err := database.Open(context.Background(), config.Config{
		DatabaseURL:       url,
		DBMaxOpenConns:    10,
		DBMaxIdleConns:    5,
		DBConnMaxLifetime: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func uniqueTag(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("%d-%d", os.Getpid(), time.Now().UnixNano())
}

func createTestUser(t *testing.T, db *database.Database, tag string) (uuid.UUID, string) {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("pass-"+tag), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	created, err := db.Client.User.Create().
		SetEmail("twofa+" + tag + ".local").
		SetNickname("tfa" + tag).
		SetPasswordHash(string(hash)).
		SetEmailVerificationStatus("not_required").
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() { _ = db.Client.User.DeleteOneID(created.ID).Exec(context.Background()) })
	return created.ID, "pass-" + tag
}

func TestConsumeRecoveryCodeConcurrent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	userID, _ := createTestUser(t, db, tag)
	repo := newRepository(db)

	if _, err := db.Client.RecoveryCode.Create().SetUserID(userID).SetCodeHash(TestCodeHash).Save(ctx); err != nil {
		t.Fatalf("seed recovery code: %v", err)
	}

	results := make([]bool, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			consumed, err := repo.consumeRecoveryCode(ctx, userID, TestCodeHash)
			if err != nil {
				t.Errorf("consume %d: %v", i, err)
				return
			}
			results[i] = consumed
		}(i)
	}
	wg.Wait()

	consumed := 0
	for _, ok := range results {
		if ok {
			consumed++
		}
	}
	if consumed != 1 {
		t.Fatalf("consumed = %d, want exactly 1", consumed)
	}

	remaining, err := db.Client.RecoveryCode.Query().
		Where(recoverycode.UserIDEQ(userID), recoverycode.UsedAtIsNil()).
		Count(ctx)
	if err != nil {
		t.Fatalf("count unused codes: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("unused codes = %d, want 0", remaining)
	}
}

func TestClaimTOTPCounterConcurrent(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	userID, _ := createTestUser(t, db, tag)
	repo := newRepository(db)

	results := make([]bool, 8)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claimed, err := repo.claimTOTPCounter(ctx, userID, 100)
			if err != nil {
				t.Errorf("claim %d: %v", i, err)
				return
			}
			results[i] = claimed
		}(i)
	}
	wg.Wait()

	claimed := 0
	for _, ok := range results {
		if ok {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("claimed = %d, want exactly 1", claimed)
	}

	fetched, err := db.Client.User.Get(ctx, userID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if fetched.TotpLastCounter == nil || *fetched.TotpLastCounter != 100 {
		t.Fatalf("stored counter = %v, want 100", fetched.TotpLastCounter)
	}
}

func TestClaimTOTPCounterOrdering(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	userID, _ := createTestUser(t, db, tag)
	repo := newRepository(db)

	for _, tc := range []struct {
		counter int64
		want    bool
		kind    string
	}{
		{counter: 100, want: true, kind: "first claim from nil"},
		{counter: 99, want: false, kind: "lower than stored"},
		{counter: 100, want: false, kind: "equal to stored"},
		{counter: 101, want: true, kind: "higher than stored"},
	} {
		claimed, err := repo.claimTOTPCounter(ctx, userID, tc.counter)
		if err != nil {
			t.Fatalf("claim %d (%s): %v", tc.counter, tc.kind, err)
		}
		if claimed != tc.want {
			t.Fatalf("claim %d (%s) = %v, want %v", tc.counter, tc.kind, claimed, tc.want)
		}
	}

	fetched, err := db.Client.User.Get(ctx, userID)
	if err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if fetched.TotpLastCounter == nil || *fetched.TotpLastCounter != 101 {
		t.Fatalf("stored counter = %v, want 101", fetched.TotpLastCounter)
	}

	missing, err := repo.claimTOTPCounter(ctx, uuid.New(), 500)
	if err != nil && !ent.IsNotFound(err) {
		t.Fatalf("claim for unknown user: %v", err)
	}
	if missing {
		t.Fatal("claim for unknown user = true, want false")
	}
}

func TestClaimTOTPSetupRejectedWhenFresh(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	tag := uniqueTag(t)
	userID, _ := createTestUser(t, db, tag)
	repo := newRepository(db)
	base := time.Unix(1700000001, 0)

	claimed, err := repo.claimTOTPSetup(ctx, userID, TestCodeHash, base)
	if err != nil {
		t.Fatalf("first claim: %v", err)
	}
	if !claimed {
		t.Fatal("first claim on fresh user rejected, want success")
	}

	reclaimed, err := repo.claimTOTPSetup(ctx, userID, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", base)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if reclaimed {
		t.Fatal("second claim inside reuse window accepted, want rejected")
	}

	stale, err := repo.claimTOTPSetup(ctx, userID, "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", base.Add(SetupReuseWindow+time.Second))
	if err != nil {
		t.Fatalf("stale claim: %v", err)
	}
	if !stale {
		t.Fatal("claim past reuse window rejected, want success")
	}
}
