package settings

import (
	"fmt"
	"regexp"
)

var builtinRegistry = []Group{}

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
