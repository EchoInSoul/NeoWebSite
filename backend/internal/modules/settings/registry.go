package settings

import (
	"fmt"
	"regexp"
)

var builtinRegistry = []Group{
	{
		Name: "site",
		Items: []Definition{
			{Name: "title", Type: FieldTypeString, Default: "", Public: true},
			{Name: "subtitle", Type: FieldTypeString, Default: "", Public: true},
			{Name: "description", Type: FieldTypeString, Default: "", Public: true},
			{Name: "keywords", Type: FieldTypeString, Default: "", Public: true},
			{Name: "url", Type: FieldTypeString, Default: "", Public: true},
			{Name: "block_search_engines", Type: FieldTypeBool, Default: false, Public: true},
		},
	},
	{
		Name: "register",
		Items: []Definition{
			{Name: "enabled", Type: FieldTypeBool, Default: true, Public: true},
			{Name: "default_group", Type: FieldTypeString, Default: "user"},
			{Name: "email_domain_control_enabled", Type: FieldTypeBool, Default: false},
			{Name: "allowed_email_domains", Type: FieldTypeString, Default: ""},
			{Name: "email_verification_enabled", Type: FieldTypeBool, Default: false},
		},
	},
	{
		Name: "auth",
		Items: []Definition{
			{Name: "password_requirements_enabled", Type: FieldTypeBool, Default: false},
			{Name: "password_min_length", Type: FieldTypeNumber, Default: int64(8)},
			{Name: "password_complexity", Type: FieldTypeString, Default: ""},
			{Name: "session_max_age_enabled", Type: FieldTypeBool, Default: false},
			{Name: "session_max_age_days", Type: FieldTypeNumber, Default: int64(7)},
			{Name: "totp_enabled", Type: FieldTypeBool, Default: false},
			{Name: "password_reset_enabled", Type: FieldTypeBool, Default: false},
			{Name: "captcha_enabled", Type: FieldTypeBool, Default: false},
			{Name: "captcha_provider", Type: FieldTypeString, Default: ""},
		},
	},
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func compileRegistry(groups []Group) (Registry, error) {
	registry := Registry{
		groups: make(map[string]map[string]Definition, len(groups)),
		order:  make([]string, 0, len(groups)),
	}
	for _, group := range groups {
		if !namePattern.MatchString(group.Name) {
			return Registry{}, fmt.Errorf("settings: invalid group name %q", group.Name)
		}
		if _, exists := registry.groups[group.Name]; exists {
			return Registry{}, fmt.Errorf("settings: duplicate group %q", group.Name)
		}
		items := make(map[string]Definition, len(group.Items))
		for _, item := range group.Items {
			if !namePattern.MatchString(item.Name) {
				return Registry{}, fmt.Errorf("settings: invalid item name %q in group %q", item.Name, group.Name)
			}
			if _, exists := items[item.Name]; exists {
				return Registry{}, fmt.Errorf("settings: duplicate item %q in group %q", item.Name, group.Name)
			}
			if err := checkDefault(item); err != nil {
				return Registry{}, fmt.Errorf("settings: item %q in group %q: %w", item.Name, group.Name, err)
			}
			items[item.Name] = item
		}
		registry.groups[group.Name] = items
		registry.order = append(registry.order, group.Name)
	}
	return registry, nil
}

func checkDefault(item Definition) error {
	switch item.Type {
	case FieldTypeString:
		if _, ok := item.Default.(string); !ok {
			return fmt.Errorf("default must be string, got %T", item.Default)
		}
	case FieldTypeNumber:
		if _, ok := item.Default.(int64); !ok {
			return fmt.Errorf("default must be int64, got %T", item.Default)
		}
	case FieldTypeBool:
		if _, ok := item.Default.(bool); !ok {
			return fmt.Errorf("default must be bool, got %T", item.Default)
		}
	default:
		return fmt.Errorf("unknown field type %d", item.Type)
	}
	return nil
}
