package settings

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

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

func TestSettingValueSchema(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	created, err := db.Client.SettingValue.Create().
		SetSettingGroup("site").
		SetName("title").
		SetValue(json.RawMessage(`"我的博客"`)).
		Save(ctx)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Client.SettingValue.DeleteOneID(created.ID).Exec(context.Background())
	})
	if string(created.Value) != `"我的博客"` {
		t.Fatalf("roundtrip value = %s, want %s", created.Value, `"我的博客"`)
	}

	dup, err := db.Client.SettingValue.Create().
		SetSettingGroup("site").
		SetName("title").
		SetValue(json.RawMessage(`"x"`)).
		Save(ctx)
	if err == nil {
		_ = db.Client.SettingValue.DeleteOneID(dup.ID).Exec(context.Background())
		t.Fatal("duplicate (setting_group, name) accepted, want constraint error")
	}
	if !ent.IsConstraintError(err) {
		t.Fatalf("duplicate error = %v, want constraint error", err)
	}

	shapes := map[string]json.RawMessage{
		"str":  json.RawMessage(`"x"`),
		"num":  json.RawMessage(`3`),
		"flag": json.RawMessage(`true`),
	}
	for name, raw := range shapes {
		row, err := db.Client.SettingValue.Create().
			SetSettingGroup("shapes").
			SetName(name).
			SetValue(raw).
			Save(ctx)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		t.Cleanup(func() {
			_ = db.Client.SettingValue.DeleteOneID(row.ID).Exec(context.Background())
		})
		if string(row.Value) != string(raw) {
			t.Fatalf("%s roundtrip = %s, want %s", name, row.Value, raw)
		}
	}
}
