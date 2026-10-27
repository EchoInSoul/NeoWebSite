package media

import (
	"context"
	"testing"

	"website/backend/internal/common/rbac"
	"website/backend/internal/database"
	"website/backend/internal/database/ent/mediaasset"
)

func cleanupMediaKey(t *testing.T, db *database.Database, key string) {
	t.Helper()

	t.Cleanup(func() {
		_, err := db.Client.MediaAsset.Delete().Where(mediaasset.KeyEQ(key)).Exec(context.Background())
		if err != nil {
			t.Errorf("cleanup media key: %v", err)
		}
	})
}

func TestMediaAssetTable(t *testing.T) {
	db := testDB(t)

	var count int
	if err := db.SQL().QueryRow("SELECT count(*) FROM information_schema.columns WHERE table_name = 'media_assets'").Scan(&count); err != nil {
		t.Fatalf("query columns: %v", err)
	}
	if count != 7 {
		t.Fatalf("media_assets has %d columns, want 7", count)
	}
}

func TestMediaAssetKeyIsUnique(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	key := uniqueTag(t) + ".jpg"
	cleanupMediaKey(t, db, key)

	insert := func() error {
		_, err := db.Client.MediaAsset.Create().
			SetProvider("local").
			SetKey(key).
			SetSizeBytes(1).
			SetOriginalName("x.jpg").
			Save(ctx)
		return err
	}

	if err := insert(); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := insert(); err == nil {
		t.Fatal("second insert with duplicate key must fail")
	}
}

func TestMediaAssetUploadedBySetNullOnUserDelete(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	userID := createTestUser(t, db, rbac.GroupKeyOwner)
	key := uniqueTag(t) + ".jpg"
	cleanupMediaKey(t, db, key)

	row, err := db.Client.MediaAsset.Create().
		SetProvider("local").
		SetKey(key).
		SetSizeBytes(1).
		SetOriginalName("x.jpg").
		SetUploadedBy(userID).
		Save(ctx)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	if err := db.Client.User.DeleteOneID(userID).Exec(ctx); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	got, err := db.Client.MediaAsset.Query().Where(mediaasset.IDEQ(row.ID)).Only(ctx)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if got.UploadedBy != nil {
		t.Fatalf("uploaded_by = %v, want nil", *got.UploadedBy)
	}
}

func TestMediaAssetProviderIsPlainText(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	key := uniqueTag(t) + ".jpg"
	cleanupMediaKey(t, db, key)

	if _, err := db.Client.MediaAsset.Create().
		SetProvider("some-future-provider").
		SetKey(key).
		SetSizeBytes(1).
		SetOriginalName("x.jpg").
		Save(ctx); err != nil {
		t.Fatalf("insert with future provider: %v", err)
	}
}
