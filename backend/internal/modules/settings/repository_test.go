package settings

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"website/backend/internal/database/ent/settingvalue"
)

func TestSettingRepo(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	repo := &repository{client: db.Client}

	if err := repo.upsert(ctx, "site", "title", json.RawMessage(`"我的博客"`)); err != nil {
		t.Fatalf("upsert insert: %v", err)
	}
	t.Cleanup(func() {
		_, _ = repo.delete(context.Background(), "site", "title")
		_, _ = repo.delete(context.Background(), "site", "port")
		_, _ = repo.delete(context.Background(), "other", "name")
	})

	raw, ok, err := repo.get(ctx, "site", "title")
	if err != nil || !ok || string(raw) != `"我的博客"` {
		t.Fatalf("get after insert = (%s, %v, %v)", raw, ok, err)
	}

	if _, ok, _ := repo.get(ctx, "site", "missing"); ok {
		t.Fatal("get missing item reports ok=true")
	}

	first, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("site"), settingvalue.NameEQ("title")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query after insert: %v", err)
	}

	time.Sleep(20 * time.Millisecond)

	if err := repo.upsert(ctx, "site", "title", json.RawMessage(`"新值"`)); err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	second, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("site"), settingvalue.NameEQ("title")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query after update: %v", err)
	}
	if string(second.Value) != `"新值"` {
		t.Fatalf("value after update = %s, want 新值", second.Value)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("updated_at not refreshed on conflict: %v -> %v", first.UpdatedAt, second.UpdatedAt)
	}

	if err := repo.upsert(ctx, "site", "port", json.RawMessage(`8080`)); err != nil {
		t.Fatalf("upsert port: %v", err)
	}
	if err := repo.upsert(ctx, "other", "name", json.RawMessage(`"y"`)); err != nil {
		t.Fatalf("upsert other: %v", err)
	}
	rows, err := repo.listByGroup(ctx, "site")
	if err != nil {
		t.Fatalf("listByGroup: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("listByGroup len = %d, want 2", len(rows))
	}
	for _, row := range rows {
		if row.SettingGroup != "site" {
			t.Fatalf("listByGroup leaked group %q", row.SettingGroup)
		}
	}

	n, err := repo.delete(ctx, "site", "title")
	if err != nil || n != 1 {
		t.Fatalf("delete existing = (%d, %v), want (1, nil)", n, err)
	}
	if _, ok, _ := repo.get(ctx, "site", "title"); ok {
		t.Fatal("row still present after delete")
	}
	n, err = repo.delete(ctx, "site", "title")
	if err != nil || n != 0 {
		t.Fatalf("delete missing = (%d, %v), want (0, nil)", n, err)
	}
}
