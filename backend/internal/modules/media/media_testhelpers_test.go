package media

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"website/backend/internal/common/rbac"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/database/ent"
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

func createTestUser(t *testing.T, db *database.Database, groupKey string) uuid.UUID {
	t.Helper()

	groupID := rbac.GroupIDUser
	switch groupKey {
	case rbac.GroupKeyOwner:
		groupID = rbac.GroupIDOwner
	case rbac.GroupKeyAdmin:
		groupID = rbac.GroupIDAdmin
	case rbac.GroupKeyUser:
	default:
		t.Fatalf("unknown group key %q", groupKey)
	}

	tag := uniqueTag(t)
	row, err := db.Client.User.Create().
		SetEmail("media-" + tag + "@example.com").
		SetNickname("media-" + tag).
		SetPasswordHash("x").
		SetEmailVerificationStatus("not_required").
		SetGroupID(groupID).
		Save(context.Background())
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	t.Cleanup(func() {
		err := db.Client.User.DeleteOneID(row.ID).Exec(context.Background())
		if err != nil && !ent.IsNotFound(err) {
			t.Errorf("cleanup user: %v", err)
		}
	})
	return row.ID
}
