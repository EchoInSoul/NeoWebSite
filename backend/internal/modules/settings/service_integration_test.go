package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"website/backend/internal/database"
	"website/backend/internal/database/ent/settingvalue"
)

func newIntegrationService(t *testing.T, groups []Group) (*Service, *database.Database) {
	t.Helper()
	db := testDB(t)
	names := groupNames(groups)
	deleteGroups(t, db, names...)
	svc, err := newServiceWith(db, groups)
	if err != nil {
		t.Fatalf("newServiceWith: %v", err)
	}
	seedGroupDefaults(t, svc, groups)
	t.Cleanup(func() {
		deleteGroups(t, db, names...)
		builtin := make([]Group, 0, len(groups))
		for _, group := range groups {
			if _, ok := builtinGroup(group.Name); ok {
				builtin = append(builtin, group)
			}
		}
		if len(builtin) == 0 {
			return
		}
		if err := writeGroupDefaults(svc, builtin); err != nil {
			t.Errorf("reseed %v: %v", names, err)
		}
	})
	return svc, db
}

func groupNames(groups []Group) []string {
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func deleteGroups(t *testing.T, db *database.Database, names ...string) {
	t.Helper()
	ctx := context.Background()
	for _, name := range names {
		_, _ = db.Client.SettingValue.Delete().Where(settingvalue.SettingGroupEQ(name)).Exec(ctx)
	}
}

func seedGroupDefaults(t *testing.T, svc *Service, groups []Group) {
	t.Helper()
	if err := writeGroupDefaults(svc, groups); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}
}

func writeGroupDefaults(svc *Service, groups []Group) error {
	ctx := context.Background()
	for _, group := range groups {
		for _, item := range group.Items {
			var err error
			switch item.Type {
			case FieldTypeString:
				err = svc.SetString(ctx, group.Name, item.Name, item.Default.(string))
			case FieldTypeNumber:
				err = svc.SetNumber(ctx, group.Name, item.Name, item.Default.(int64))
			case FieldTypeBool:
				err = svc.SetBool(ctx, group.Name, item.Name, item.Default.(bool))
			default:
				return fmt.Errorf("unknown field type %d", item.Type)
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func TestServiceWriteReadRoundtrip(t *testing.T) {
	svc, _ := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetString(ctx, "demo", "title", "新标题"); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	if err := svc.SetNumber(ctx, "demo", "port", 9090); err != nil {
		t.Fatalf("SetNumber: %v", err)
	}
	if err := svc.SetBool(ctx, "demo", "maintenance", true); err != nil {
		t.Fatalf("SetBool: %v", err)
	}
	if got, err := svc.GetString(ctx, "demo", "title"); err != nil || got != "新标题" {
		t.Fatalf("GetString = (%q, %v), want 新标题", got, err)
	}
	if got, err := svc.GetNumber(ctx, "demo", "port"); err != nil || got != 9090 {
		t.Fatalf("GetNumber = (%d, %v), want 9090", got, err)
	}
	if got, err := svc.GetBool(ctx, "demo", "maintenance"); err != nil || !got {
		t.Fatalf("GetBool = (%v, %v), want true", got, err)
	}
}

func TestServiceDefaultRead(t *testing.T) {
	svc, _ := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if got, err := svc.GetString(ctx, "demo", "title"); err != nil || got != "我的博客" {
		t.Fatalf("GetString default = (%q, %v)", got, err)
	}
	if got, err := svc.GetNumber(ctx, "demo", "port"); err != nil || got != 8080 {
		t.Fatalf("GetNumber default = (%d, %v)", got, err)
	}
	if got, err := svc.GetBool(ctx, "demo", "maintenance"); err != nil || got {
		t.Fatalf("GetBool default = (%v, %v)", got, err)
	}
}

func TestServiceResetItem(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()
	repo := &repository{client: db.Client}

	if err := svc.SetString(ctx, "demo", "title", "覆盖"); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	if err := svc.ResetItem(ctx, "demo", "title"); err != nil {
		t.Fatalf("ResetItem existing row: %v", err)
	}
	if raw, ok, err := repo.get(ctx, "demo", "title"); err != nil || !ok || string(raw) != `"我的博客"` {
		t.Fatalf("row after ResetItem = (%s, %v, %v), want the default value", raw, ok, err)
	}
	if got, err := svc.GetString(ctx, "demo", "title"); err != nil || got != "我的博客" {
		t.Fatalf("GetString after reset = (%q, %v)", got, err)
	}

	if err := svc.ResetItem(ctx, "demo", "port"); err != nil {
		t.Fatalf("ResetItem never-written item = %v, want idempotent success", err)
	}
	if err := svc.ResetItem(ctx, "demo", "ghost"); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("ResetItem unknown item = %v, want ErrUnknownItem", err)
	}
	if err := svc.ResetItem(ctx, "nope", "title"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("ResetItem unknown group = %v, want ErrUnknownGroup", err)
	}
}

func TestServiceGroupMerge(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetString(ctx, "demo", "title", "覆盖标题"); err != nil {
		t.Fatalf("SetString: %v", err)
	}
	values, err := svc.GetGroup(ctx, "demo")
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

	if err := svc.SetNumber(ctx, "demo", "port", 8080); err != nil {
		t.Fatalf("SetNumber: %v", err)
	}
	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "3.5", "demo", "port"); err != nil {
		t.Fatalf("plant bad row: %v", err)
	}
	_, err = svc.GetGroup(ctx, "demo")
	if err == nil {
		t.Fatal("GetGroup with bad row succeeded, want error")
	}
	if !strings.Contains(err.Error(), "demo.port") {
		t.Fatalf("GetGroup with bad row = %v, want error naming demo.port", err)
	}
	if errors.Is(err, ErrTypeMismatch) {
		t.Fatal("corrupt stored value must not be reported as ErrTypeMismatch")
	}
}

func TestServiceSetGroupAtomic(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()
	repo := &repository{client: db.Client}

	err := svc.SetGroup(ctx, "demo", []ItemWrite{
		{Name: "title", Type: FieldTypeString, Value: "整组标题"},
		{Name: "ghost", Type: FieldTypeString, Value: "x"},
	})
	if !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("SetGroup with unknown item = %v, want ErrUnknownItem", err)
	}
	if raw, ok, _ := repo.get(ctx, "demo", "title"); !ok || string(raw) != `"我的博客"` {
		t.Fatalf("bad SetGroup changed title to %s, want the seeded default", raw)
	}

	err = svc.SetGroup(ctx, "demo", []ItemWrite{
		{Name: "port", Type: FieldTypeNumber, Value: int64(1)},
		{Name: "maintenance", Type: FieldTypeBool, Value: "x"},
	})
	if !errors.Is(err, ErrTypeMismatch) {
		t.Fatalf("SetGroup with type mismatch = %v, want ErrTypeMismatch", err)
	}
	if raw, ok, _ := repo.get(ctx, "demo", "port"); !ok || string(raw) != `8080` {
		t.Fatalf("type-mismatch SetGroup changed port to %s, want the seeded 8080", raw)
	}

	if err := svc.SetGroup(ctx, "demo", []ItemWrite{
		{Name: "title", Type: FieldTypeString, Value: "整组标题"},
		{Name: "port", Type: FieldTypeNumber, Value: int64(1234)},
		{Name: "maintenance", Type: FieldTypeBool, Value: true},
	}); err != nil {
		t.Fatalf("SetGroup happy path: %v", err)
	}
	values, err := svc.GetGroup(ctx, "demo")
	if err != nil {
		t.Fatalf("GetGroup after SetGroup: %v", err)
	}
	if values["title"] != "整组标题" || values["port"] != int64(1234) || values["maintenance"] != true {
		t.Fatalf("SetGroup values wrong: %+v", values)
	}
}

func TestServiceBadRowReturnsError(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetNumber(ctx, "demo", "port", 9090); err != nil {
		t.Fatalf("SetNumber: %v", err)
	}
	if err := svc.SetString(ctx, "demo", "title", "标题"); err != nil {
		t.Fatalf("SetString: %v", err)
	}

	for name, raw := range map[string]string{
		"decimal":  "3.5",
		"overflow": "9223372036854775808",
	} {
		if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", raw, "demo", "port"); err != nil {
			t.Fatalf("plant %s bad row: %v", name, err)
		}
		if got, err := svc.GetNumber(ctx, "demo", "port"); err == nil {
			t.Fatalf("GetNumber with %s bad row = %d, want error", name, got)
		} else if got != 0 {
			t.Fatalf("GetNumber with %s bad row = %d, want zero value", name, got)
		} else if !strings.Contains(err.Error(), "settings: demo.port") {
			t.Fatalf("GetNumber with %s bad row = %v, want error naming the item", name, err)
		}
	}

	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "1e3", "demo", "port"); err != nil {
		t.Fatalf("plant normalized row: %v", err)
	}
	if got, err := svc.GetNumber(ctx, "demo", "port"); err != nil || got != 1000 {
		t.Fatalf("GetNumber with normalized row = (%d, %v), want 1000 (jsonb normalizes 1e3)", got, err)
	}

	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "3", "demo", "title"); err != nil {
		t.Fatalf("plant type-mismatch row: %v", err)
	}
	if got, err := svc.GetString(ctx, "demo", "title"); err == nil {
		t.Fatalf("GetString with type-mismatch row = %q, want error", got)
	} else if !strings.Contains(err.Error(), "settings: demo.title") {
		t.Fatalf("GetString with type-mismatch row = %v, want error naming the item", err)
	}

	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "null", "demo", "title"); err != nil {
		t.Fatalf("plant null row (string): %v", err)
	}
	if got, err := svc.GetString(ctx, "demo", "title"); err == nil {
		t.Fatalf("GetString with null row = %q, want error", got)
	}

	if err := svc.SetBool(ctx, "demo", "maintenance", true); err != nil {
		t.Fatalf("SetBool: %v", err)
	}
	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "null", "demo", "maintenance"); err != nil {
		t.Fatalf("plant null row (bool): %v", err)
	}
	if got, err := svc.GetBool(ctx, "demo", "maintenance"); err == nil {
		t.Fatalf("GetBool with null row = %v, want error", got)
	} else if got {
		t.Fatal("GetBool with null row returned true, want zero value")
	}
}

func TestPatchGroupRepairsCorruptValue(t *testing.T) {
	svc, db := newIntegrationService(t, builtinRegistry)
	ctx := context.Background()

	if err := svc.SetNumber(ctx, "auth", "session_max_age_days", 7); err != nil {
		t.Fatalf("SetNumber: %v", err)
	}
	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", `"abc"`, "auth", "session_max_age_days"); err != nil {
		t.Fatalf("plant bad row: %v", err)
	}
	if _, err := svc.GetNumber(ctx, "auth", "session_max_age_days"); err == nil {
		t.Fatal("GetNumber with corrupt row succeeded, want error")
	}

	values, err := svc.PatchGroup(ctx, "auth", map[string]json.RawMessage{
		"session_max_age_days": json.RawMessage(`14`),
	})
	if err != nil {
		t.Fatalf("PatchGroup repair = %v, want success", err)
	}
	if values["session_max_age_days"] != int64(14) {
		t.Fatalf("repaired days = %v, want 14", values["session_max_age_days"])
	}
	if got, err := svc.GetNumber(ctx, "auth", "session_max_age_days"); err != nil || got != 14 {
		t.Fatalf("GetNumber after repair = (%d, %v), want 14", got, err)
	}
}

func TestPatchGroupRejectsCorruptSiblingBeforeWrite(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetNumber(ctx, "demo", "port", 8080); err != nil {
		t.Fatalf("SetNumber: %v", err)
	}
	if _, err := db.SQL().Exec("UPDATE setting_values SET value = $1::jsonb WHERE setting_group = $2 AND name = $3", "3.5", "demo", "port"); err != nil {
		t.Fatalf("plant bad row: %v", err)
	}

	if _, err := svc.PatchGroup(ctx, "demo", map[string]json.RawMessage{"title": json.RawMessage(`"新标题"`)}); err == nil {
		t.Fatal("PatchGroup with corrupt sibling succeeded, want error")
	}
	if raw, ok, _ := (&repository{client: db.Client}).get(ctx, "demo", "title"); !ok || string(raw) != `"我的博客"` {
		t.Fatalf("failed PatchGroup wrote title = %s, want the seeded default", raw)
	}

	values, err := svc.PatchGroup(ctx, "demo", map[string]json.RawMessage{"port": json.RawMessage(`1234`)})
	if err != nil {
		t.Fatalf("PatchGroup repair = %v, want success", err)
	}
	if values["port"] != int64(1234) {
		t.Fatalf("repaired port = %v, want 1234", values["port"])
	}
}

func TestServicePublicSettings(t *testing.T) {
	groups := []Group{
		{
			Name:  "demo",
			Items: []Definition{{Name: "title", Type: FieldTypeString, Default: "我的博客"}},
		},
		{
			Name: "open",
			Items: []Definition{
				{Name: "flag", Type: FieldTypeBool, Default: true, Public: true},
				{Name: "hidden", Type: FieldTypeString, Default: "secret"},
			},
		},
		{
			Name:  "extra",
			Items: []Definition{{Name: "visible", Type: FieldTypeNumber, Default: int64(7), Public: true}},
		},
	}
	svc, _ := newIntegrationService(t, groups)
	ctx := context.Background()

	listed, err := svc.PublicSettings(ctx)
	if err != nil {
		t.Fatalf("PublicSettings = %v, want no error", err)
	}
	if len(listed) != 2 {
		t.Fatalf("PublicSettings = %#v, want the two groups carrying a public item", listed)
	}
	if listed[0].Group != "open" || listed[1].Group != "extra" {
		t.Fatalf("group order = %v, want [open extra] following the registry order", listed)
	}
	if listed[0].Values["flag"] != true || len(listed[0].Values) != 1 {
		t.Fatalf("open values = %v, want flag=true and no private item", listed[0].Values)
	}

	if err := svc.SetBool(ctx, "open", "flag", false); err != nil {
		t.Fatalf("SetBool flag: %v", err)
	}
	if err := svc.SetString(ctx, "open", "hidden", "written"); err != nil {
		t.Fatalf("SetString hidden: %v", err)
	}
	listed, err = svc.PublicSettings(ctx)
	if err != nil {
		t.Fatalf("PublicSettings after writes = %v", err)
	}
	if listed[0].Values["flag"] != false || len(listed[0].Values) != 1 {
		t.Fatalf("open values after writes = %v, want flag=false and still no private item", listed[0].Values)
	}
}

func TestServiceMissingRowReturnsError(t *testing.T) {
	groups := append(sampleGroups(), Group{
		Name:  "extra",
		Items: []Definition{{Name: "flag", Type: FieldTypeBool, Default: true}},
	})
	svc, db := newIntegrationService(t, groups)
	ctx := context.Background()

	if _, err := db.Client.SettingValue.Delete().
		Where(settingvalue.SettingGroupEQ("extra"), settingvalue.NameEQ("flag")).
		Exec(ctx); err != nil {
		t.Fatalf("delete extra.flag: %v", err)
	}

	if _, err := svc.GetBool(ctx, "extra", "flag"); err == nil {
		t.Fatal("GetBool without a row succeeded, want error")
	} else if !strings.Contains(err.Error(), "extra.flag") {
		t.Fatalf("GetBool without a row = %v, want error naming extra.flag", err)
	}
	if _, err := svc.GetGroup(ctx, "extra"); err == nil {
		t.Fatal("GetGroup without a row succeeded, want error")
	}
}

func TestServiceResetItemWritesDefault(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if _, err := db.Client.SettingValue.Delete().
		Where(settingvalue.SettingGroupEQ("demo"), settingvalue.NameEQ("port")).
		Exec(ctx); err != nil {
		t.Fatalf("delete demo.port: %v", err)
	}
	if _, err := svc.GetNumber(ctx, "demo", "port"); err == nil {
		t.Fatal("GetNumber without a row succeeded, want error")
	}

	if err := svc.ResetItem(ctx, "demo", "port"); err != nil {
		t.Fatalf("ResetItem: %v", err)
	}
	if got, err := svc.GetNumber(ctx, "demo", "port"); err != nil || got != 8080 {
		t.Fatalf("GetNumber after ResetItem = (%d, %v), want 8080", got, err)
	}
}

func TestServiceUpdatedAtRefresh(t *testing.T) {
	svc, db := newIntegrationService(t, sampleGroups())
	ctx := context.Background()

	if err := svc.SetString(ctx, "demo", "title", "第一次"); err != nil {
		t.Fatalf("SetString 1: %v", err)
	}
	first, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("demo"), settingvalue.NameEQ("title")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query first: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if err := svc.SetString(ctx, "demo", "title", "第二次"); err != nil {
		t.Fatalf("SetString 2: %v", err)
	}
	second, err := db.Client.SettingValue.Query().
		Where(settingvalue.SettingGroupEQ("demo"), settingvalue.NameEQ("title")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query second: %v", err)
	}
	if !second.UpdatedAt.After(first.UpdatedAt) {
		t.Fatalf("updated_at not refreshed: %v -> %v", first.UpdatedAt, second.UpdatedAt)
	}
	if got, _ := svc.GetString(ctx, "demo", "title"); got != "第二次" {
		t.Fatalf("value after second write = %q", got)
	}
}
