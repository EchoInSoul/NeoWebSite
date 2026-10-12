package rbac

import (
	"strings"

	"github.com/google/uuid"
)

const (
	PermissionUsersView    = "system.users.view"
	PermissionUsersEdit    = "system.users.edit"
	PermissionGroupsView   = "system.groups.view"
	PermissionGroupsEdit   = "system.groups.edit"
	PermissionSettingsView = "system.settings.view"
	PermissionSettingsEdit = "system.settings.edit"
)

const (
	GroupKeyOwner = "owner"
	GroupKeyAdmin = "admin"
	GroupKeyUser  = "user"
)

var (
	GroupIDOwner = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	GroupIDAdmin = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	GroupIDUser  = uuid.MustParse("00000000-0000-4000-8000-000000000003")
)

var impliedActions = map[string][]string{
	"edit": {"view"},
}

type Permission struct {
	Key  string
	Name string
}

type PresetGroup struct {
	ID   uuid.UUID
	Key  string
	Name string
}

var catalog = []Permission{
	{Key: PermissionUsersView, Name: "查看用户"},
	{Key: PermissionUsersEdit, Name: "管理用户"},
	{Key: PermissionGroupsView, Name: "查看用户组"},
	{Key: PermissionGroupsEdit, Name: "管理用户组"},
	{Key: PermissionSettingsView, Name: "查看设置"},
	{Key: PermissionSettingsEdit, Name: "管理设置"},
}

var presetGroups = []PresetGroup{
	{ID: GroupIDOwner, Key: GroupKeyOwner, Name: "所有者"},
	{ID: GroupIDAdmin, Key: GroupKeyAdmin, Name: "管理员"},
	{ID: GroupIDUser, Key: GroupKeyUser, Name: "普通用户"},
}

func Catalog() []Permission {
	out := make([]Permission, len(catalog))
	copy(out, catalog)
	return out
}

func All() []string {
	out := make([]string, 0, len(catalog))
	for _, permission := range catalog {
		out = append(out, permission.Key)
	}
	return out
}

func IsValid(key string) bool {
	for _, permission := range catalog {
		if permission.Key == key {
			return true
		}
	}
	return false
}

func PresetGroups() []PresetGroup {
	out := make([]PresetGroup, len(presetGroups))
	copy(out, presetGroups)
	return out
}

func PresetGroupByKey(key string) (PresetGroup, bool) {
	for _, group := range presetGroups {
		if group.Key == key {
			return group, true
		}
	}
	return PresetGroup{}, false
}

func IsPresetGroupKey(key string) bool {
	_, ok := PresetGroupByKey(key)
	return ok
}

func PresetPermissions(systemKey string) []string {
	switch systemKey {
	case GroupKeyOwner, GroupKeyAdmin:
		return All()
	default:
		return []string{}
	}
}

func Expand(keys []string) []string {
	out := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	for _, key := range keys {
		separator := strings.LastIndex(key, ".")
		if separator < 0 {
			continue
		}
		module := key[:separator]
		for _, implied := range impliedActions[key[separator+1:]] {
			candidate := module + "." + implied
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	return out
}
