package access

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"website/backend/internal/common/rbac"
	"website/backend/internal/config"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/group"
	"website/backend/internal/database/ent/grouppermission"
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
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func TestMigrationSeed(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	for _, preset := range rbac.PresetGroups() {
		row, err := db.Client.Group.Query().Where(group.IDEQ(preset.ID)).Only(ctx)
		if err != nil {
			t.Fatalf("preset group %q missing: %v", preset.Key, err)
		}
		if row.SystemKey != preset.Key {
			t.Fatalf("preset group %q has system_key %q", preset.Key, row.SystemKey)
		}
		if row.Name != preset.Name {
			t.Fatalf("preset group %q has name %q, want %q", preset.Key, row.Name, preset.Name)
		}
		count, err := db.Client.GroupPermission.Query().Where(grouppermission.GroupIDEQ(preset.ID)).Count(ctx)
		if err != nil {
			t.Fatalf("count preset permissions: %v", err)
		}
		if count != 0 {
			t.Fatalf("preset group %q has %d permission rows, want 0", preset.Key, count)
		}
	}

	tag := uniqueTag(t)
	created, err := db.Client.User.Create().
		SetEmail("user+" + tag + "@test.local").
		SetNickname("user" + tag).
		SetPasswordHash("hash-" + tag).
		Save(ctx)
	if err != nil {
		t.Fatalf("create user without explicit group: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.User.DeleteOneID(created.ID).Exec(context.Background())
	})
	if created.GroupID != rbac.GroupIDUser {
		t.Fatalf("new user landed in group %s, want %s", created.GroupID, rbac.GroupIDUser)
	}
}
