package settings

import (
	"context"
	"errors"
	"testing"
	"time"

	"website/backend/internal/database"
	"website/backend/internal/database/ent/settingvalue"
)

func newIntegrationService(t *testing.T, groups []Group) (*Service, *database.Database) {
	t.Helper()
	db := testDB(t)
	resetGroups(t, db, groupNames(groups)...)
	svc, err := newServiceWith(db, groups, testLogger())
	if err != nil {
		t.Fatalf("newServiceWith: %v", err)
	}
	return svc, db
}

func groupNames(groups []Group) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func resetGroups(t *testing.T, db *database.Database, names ...string) {
	t.Helper()
	cleanup := func() {
		ctx := context.Background()
		for _, name := range names {
			_, _ = db.Client.SettingValue.Delete().Where(settingvalue.SettingGroupEQ(name)).Exec(ctx)
		}
	}
	cleanup()
	t.Cleanup(cleanup)
}

func TestServiceWriteReadRoundtrip(t *testing.T) {
	svc, _ := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetString(ctx, "site", "title", "新标题"); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	if err := svc.SetNumber(ctx, "site", "port", 9090); err != nil {
		t.Fatalf("SetNumber: %v", err)
	}
	if err := svc.SetBool(ctx, "site", "maintenance", true); err != nil {
		t.Fatalf("SetBool: %v", err)
	}
	if got, err := svc.GetString(ctx, "site", "title"); err != nil || got != "新标题" {
		t.Fatalf("GetString = (%q, %v), want 新标题", got, err)
	}
	if got, err := svc.GetNumber(ctx, "site", "port"); err != nil || got != 9090 {
		t.Fatalf("GetNumber = (%d, %v), want 9090", got, err)
	}
	if got, err := svc.GetBool(ctx, "site", "maintenance"); err != nil || !got {
		t.Fatalf("GetBool = (%v, %v), want true", got, err)
	}
}

func TestServiceDefaultRead(t *testing.T) {
	svc, _ := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if got, err := svc.GetString(ctx, "site", "title"); err != nil || got != "我的博客" {
		t.Fatalf("GetString default = (%q, %v)", got, err)
	}
	if got, err := svc.GetNumber(ctx, "site", "port"); err != nil || got != 8080 {
		t.Fatalf("GetNumber default = (%d, %v)", got, err)
	}
	if got, err := svc.GetBool(ctx, "site", "maintenance"); err != nil || got {
		t.Fatalf("GetBool default = (%v, %v)", got, err)
	}
}

func TestServiceResetItem(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()
	repo := &repository{client: db.Client}

	if err := svc.SetString(ctx, "site", "title", "覆盖"); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	if err := svc.ResetItem(ctx, "site", "title"); err != nil {
		t.Fatalf("ResetItem existing row: %v", err)
	}
	if _, ok, _ := repo.get(ctx, "site", "title"); ok {
		t.Fatal("row still present after ResetItem")
	}
	if got, err := svc.GetString(ctx, "site", "title"); err != nil || got != "我的博客" {
		t.Fatalf("GetString after reset = (%q, %v)", got, err)
	}

	if err := svc.ResetItem(ctx, "site", "port"); err != nil {
		t.Fatalf("ResetItem never-written item = %v, want idempotent success", err)
	}
	if err := svc.ResetItem(ctx, "site", "ghost"); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("ResetItem unknown item = %v, want ErrUnknownItem", err)
	}
	if err := svc.ResetItem(ctx, "nope", "title"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("ResetItem unknown group = %v, want ErrUnknownGroup", err)
	}
}

func TestServiceGroupMerge(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetString(ctx, "site", "title", "覆盖标题"); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	values, err := svc.GetGroup(ctx, "site")
	if err != nil {
		t.Fatalf("GetGroup: %v", err)
	}
	if len(values) != 3 {
		t.Fatalf("GetGroup len = %d, want 3", len(values))
	}
	if values["title"] != "覆盖标题" {
		t.Fatalf("merged title = %v, want 覆盖标题", values["title"])
	}
	if values["port"] != int64(8080) {
		t.Fatalf("merged port = %v, want 8080", values["port"])
	}
	if values["maintenance"] != false {
		t.Fatalf("merged maintenance = %v, want false", values["maintenance"])
	}

	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "3.5", "site", "port"); err != nil {
		t.Fatalf("plant bad row: %v", err)
	}
	values, err = svc.GetGroup(ctx, "site")
	if err != nil {
		t.Fatalf("GetGroup with bad row = %v, want nil error", err)
	}
	if values["port"] != int64(8080) {
		t.Fatalf("bad-row port = %v, want default 8080", values["port"])
	}
	if values["title"] != "覆盖标题" {
		t.Fatalf("bad-row poisoned other items: title = %v", values["title"])
	}
}

func TestServiceSetGroupAtomic(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()
	repo := &repository{client: db.Client}

	err := svc.SetGroup(ctx, "site", []ItemWrite{
		{Name: "title", Type: FieldTypeString, Value: "整组标题"},
		{Name: "ghost", Type: FieldTypeString, Value: "x"},
	})
	if !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("SetGroup with unknown item = %v, want ErrUnknownItem", err)
	}
	if _, ok, _ := repo.get(ctx, "site", "title"); ok {
		t.Fatal("bad SetGroup left a row behind, want zero writes")
	}

	err = svc.SetGroup(ctx, "site", []ItemWrite{
		{Name: "port", Type: FieldTypeNumber, Value: int64(1)},
		{Name: "maintenance", Type: FieldTypeBool, Value: "x"},
	})
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("SetGroup with type mismatch = %v, want ErrTypeMismatch", err)
	}
	if _, ok, _ := repo.get(ctx, "site", "port"); ok {
		t.Fatal("type-mismatch SetGroup left a row behind, want zero writes")
	}

	if err := svc.SetGroup(ctx, "site", []ItemWrite{
		{Name: "title", Type: FieldTypeString, Value: "整组标题"},
		{Name: "port", Type: FieldTypeNumber, Value: int64(1234)},
		{Name: "maintenance", Type: FieldTypeBool, Value: true},
	}); err != nil {
		t.Fatalf("SetGroup happy path: %v", err)
	}
	values, err := svc.GetGroup(ctx, "site")
	if err != nil {
		t.Fatalf("GetGroup after SetGroup: %v", err)
	}
	if values["title"] != "整组标题" || values["port"] != int64(1234) || values["maintenance"] != true {
		t.Fatalf("SetGroup values wrong: %+v", values)
	}
}

func TestServiceBadRowFallback(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetNumber(ctx, "site", "port", 9090); err != nil {
		t.Fatalf("SetNumber: %v", err)
	}
	if err := svc.SetString(ctx, "site", "title", "标题"); err != nil {
		t.Fatalf("SetString: %v", err)
	}

	for name, raw := range map[string]string{
		"decimal":  "3.5",
		"overflow": "9223372036854775808",
	} {
		if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", raw, "site", "port"); err != nil {
			t.Fatalf("plant %s bad row: %v", name, err)
		}
		got, err := svc.GetNumber(ctx, "site", "port")
		if err != nil || got != 8080 {
			t.Fatalf("GetNumber with %s bad row = (%d, %v), want default 8080 no error", name, got, err)
		}
	}

	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "1e3", "site", "port"); err != nil {
		t.Fatalf("plant scientific row: %v", err)
	}
	if got, err := svc.GetNumber(ctx, "site", "port"); err != nil || got != 1000 {
		t.Fatalf("GetNumber with scientific row = (%d, %v), want 1000 (jsonb normalizes 1e3)", got, err)
	}

	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "3", "site", "title"); err != nil {
		t.Fatalf("plant type-mismatch row: %v", err)
	}
	if got, err := svc.GetString(ctx, "site", "title"); err != nil || got != "我的博客" {
		t.Fatalf("GetString with type-mismatch row = (%q, %v), want default no error", got, err)
	}
}

func TestServiceExtraGroupNoMigration(t *testing.T) {
	groups := append(sampleGroups(), Group{
		Name:  "extra",
		Items: []Definition{{Name: "flag", Type: FieldTypeBool, Default: true}},
	})
	svc, _ := newIntegrationService(t, groups)
	ctx := context.Background()

	if got, err := svc.GetBool(ctx, "extra", "flag"); err != nil || !got {
		t.Fatalf("GetBool extra group = (%v, %v), want default true", got, err)
	}
}

func TestServiceUpdatedAtRefresh(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetString(ctx, "site", "title", "第一次"); err != nil {
		t.Fatalf("SetString 1: %v", err)
	}
	first, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("site"), settingvalue.NameEQ("title")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query first: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := svc.SetString(ctx, "site", "title", "第二次"); err != nil {
		t.Fatalf("SetString 2: %v", err)
	}
	second, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("site"), settingvalue.NameEQ("title")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query second: %v", err)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("updated_at not refreshed: %v -> %v", first.UpdatedAt, second.UpdatedAt)
	}
	if got, _ := svc.GetString(ctx, "site", "title"); got != "第二次" {
		t.Fatalf("value after second write = %q", got)
	}
}
