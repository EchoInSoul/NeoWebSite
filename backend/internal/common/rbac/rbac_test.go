package rbac

import (
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestCatalogMatchesFixedValues(t *testing.T) {
	want := []Permission{
		{Key: PermissionUsersView, Name: "查看用户"},
		{Key: PermissionUsersEdit, Name: "管理用户"},
		{Key: PermissionGroupsView, Name: "查看用户组"},
		{Key: PermissionGroupsEdit, Name: "管理用户组"},
	}
	if got := Catalog(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Catalog() = %#v, want %#v", got, want)
	}
	if PermissionUsersView != "system.users.view" ||
		PermissionUsersEdit != "system.users.edit" ||
		PermissionGroupsView != "system.groups.view" ||
		PermissionGroupsEdit != "system.groups.edit" {
		t.Fatal("permission key constants drifted from the fixed keys")
	}
}

func TestAllMatchesCatalogOrder(t *testing.T) {
	catalog := Catalog()
	all := All()
	if len(all) != len(catalog) {
		t.Fatalf("All() has %d entries, want %d", len(all), len(catalog))
	}
	for i, permission := range catalog {
		if all[i] != permission.Key {
			t.Fatalf("All()[%d] = %q, want %q", i, all[i], permission.Key)
		}
	}
}

func TestIsValidRejectsUnknownKeys(t *testing.T) {
	for _, permission := range Catalog() {
		if !IsValid(permission.Key) {
			t.Fatalf("IsValid(%q) = false, want true", permission.Key)
		}
	}
	for _, key := range []string{"", "system.users.remove", "content.posts.edit", "system.users.view "} {
		if IsValid(key) {
			t.Fatalf("IsValid(%q) = true, want false", key)
		}
	}
}

func TestPresetGroupsAreFixed(t *testing.T) {
	groups := PresetGroups()
	want := []PresetGroup{
		{ID: GroupIDOwner, Key: GroupKeyOwner, Name: "所有者"},
		{ID: GroupIDAdmin, Key: GroupKeyAdmin, Name: "管理员"},
		{ID: GroupIDUser, Key: GroupKeyUser, Name: "普通用户"},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("PresetGroups() = %#v, want %#v", groups, want)
	}
	if GroupIDOwner != uuid.MustParse("00000000-0000-4000-8000-000000000001") ||
		GroupIDAdmin != uuid.MustParse("00000000-0000-4000-8000-000000000002") ||
		GroupIDUser != uuid.MustParse("00000000-0000-4000-8000-000000000003") {
		t.Fatal("preset group ids drifted from the fixed values")
	}
	if GroupKeyOwner != "owner" || GroupKeyAdmin != "admin" || GroupKeyUser != "user" {
		t.Fatal("preset group keys drifted from the fixed values")
	}
	seen := map[uuid.UUID]bool{}
	for _, group := range groups {
		if seen[group.ID] {
			t.Fatalf("duplicate preset group id %s", group.ID)
		}
		seen[group.ID] = true
	}
	for _, group := range groups {
		found, ok := PresetGroupByKey(group.Key)
		if !ok || found != group {
			t.Fatalf("PresetGroupByKey(%q) = %#v, %v, want %#v, true", group.Key, found, ok, group)
		}
		if !IsPresetGroupKey(group.Key) {
			t.Fatalf("IsPresetGroupKey(%q) = false, want true", group.Key)
		}
	}
	if IsPresetGroupKey("editor") {
		t.Fatal("IsPresetGroupKey(editor) = true, want false")
	}
	if _, ok := PresetGroupByKey("editor"); ok {
		t.Fatal("PresetGroupByKey(editor) reported a group, want none")
	}
}

func TestPresetPermissions(t *testing.T) {
	all := All()
	for _, key := range []string{GroupKeyOwner, GroupKeyAdmin} {
		if got := PresetPermissions(key); !reflect.DeepEqual(got, all) {
			t.Fatalf("PresetPermissions(%q) = %#v, want %#v", key, got, all)
		}
	}
	for _, key := range []string{GroupKeyUser, "editor", ""} {
		if got := PresetPermissions(key); len(got) != 0 {
			t.Fatalf("PresetPermissions(%q) = %#v, want empty", key, got)
		}
	}
}

func TestExpandAddsImpliedView(t *testing.T) {
	got := Expand([]string{PermissionUsersEdit, PermissionGroupsEdit})
	want := []string{PermissionUsersEdit, PermissionGroupsEdit, PermissionUsersView, PermissionGroupsView}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Expand(edits) = %#v, want %#v", got, want)
	}
	got = Expand([]string{PermissionUsersView})
	if !reflect.DeepEqual(got, []string{PermissionUsersView}) {
		t.Fatalf("Expand(view only) = %#v, want unchanged", got)
	}
	got = Expand([]string{PermissionUsersEdit, PermissionUsersView})
	if want := []string{PermissionUsersEdit, PermissionUsersView}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Expand(edit+view) = %#v, want %#v", got, want)
	}
	if got := Expand(nil); len(got) != 0 {
		t.Fatalf("Expand(nil) = %#v, want empty", got)
	}
}
