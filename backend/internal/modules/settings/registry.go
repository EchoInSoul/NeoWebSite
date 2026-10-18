package settings

import (
	"fmt"
	"regexp"
	"strings"

	"website/backend/internal/common/apierror"
	"website/backend/internal/common/rbac"
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
			{Name: "default_group", Type: FieldTypeString, Default: rbac.PresetGroupNameByKey(rbac.GroupKeyUser)},
			{Name: "email_domain_control_enabled", Type: FieldTypeBool, Default: false},
			{Name: "allowed_email_domains", Type: FieldTypeString, Default: ""},
			{Name: "email_verification_enabled", Type: FieldTypeBool, Default: false},
		},
	},
	{
		Name:       "auth",
		ValidateOn: []string{"password_min_length", "session_max_age_days", "session_max_age_enabled"},
		Validate:   validateAuthGroup,
		Items: []Definition{
			{Name: "password_requirements_enabled", Type: FieldTypeBool, Default: false},
			{Name: "password_min_length", Type: FieldTypeNumber, Default: int64(8)},
			{Name: "password_complexity", Type: FieldTypeString, Default: ""},
			{Name: "session_max_age_enabled", Type: FieldTypeBool, Default: false},
			{Name: "session_max_age_days", Type: FieldTypeNumber, Default: int64(7)},
			{Name: "totp_enabled", Type: FieldTypeBool, Default: true, Public: true},
			{Name: "password_reset_enabled", Type: FieldTypeBool, Default: false},
			{Name: "captcha_enabled", Type: FieldTypeBool, Default: false},
			{Name: "captcha_provider", Type: FieldTypeString, Default: ""},
		},
	},
}

var namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func compileRegistry(groups []Group) (Registry, error) {
	registry := Registry{
		groups:     make(map[string]map[string]Definition, len(groups)),
		validators: make(map[string]groupValidator, len(groups)),
		order:      make([]string, 0, len(groups)),
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
		if group.Validate != nil {
			if len(group.ValidateOn) == 0 {
				return Registry{}, fmt.Errorf("settings: group %q has a validator without items", group.Name)
			}
			for _, name := range group.ValidateOn {
				if _, ok := items[name]; !ok {
					return Registry{}, fmt.Errorf("settings: group %q validator references unknown item %q", group.Name, name)
				}
			}
			registry.validators[group.Name] = groupValidator{items: group.ValidateOn, check: group.Validate}
		}
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

const (
	passwordMinLengthFloor = 8
	passwordMinLengthCap   = 64
	sessionMaxAgeDaysFloor = 1
	sessionMaxAgeDaysCap   = 30
)

func validateAuthGroup(values map[string]any) error {
	length, ok := values["password_min_length"].(int64)
	if !ok {
		return fmt.Errorf("settings: password_min_length is not a number")
	}
	if length < passwordMinLengthFloor || length > passwordMinLengthCap {
		return &ItemError{Item: "password_min_length", Err: apierror.ErrSettingConflict}
	}

	days, ok := values["session_max_age_days"].(int64)
	if !ok {
		return fmt.Errorf("settings: session_max_age_days is not a number")
	}
	if days < sessionMaxAgeDaysFloor || days > sessionMaxAgeDaysCap {
		return &ItemError{Item: "session_max_age_days", Err: apierror.ErrSettingConflict}
	}
	return nil
}

func SplitListValue(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' })
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
