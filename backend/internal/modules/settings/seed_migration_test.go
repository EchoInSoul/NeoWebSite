package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var seedRowPattern = regexp.MustCompile(`^\(gen_random_uuid\(\), '([a-z0-9_]+)', '([a-z0-9_]+)', '(.*)'::jsonb, now\(\)\)[,;]?$`)

func TestSeedMigrationMatchesBuiltinRegistry(t *testing.T) {
	paths, err := filepath.Glob("../../../migrations/*_seed_setting_values.sql")
	if err != nil {
		t.Fatalf("glob seed migration: %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("seed migrations = %v, want exactly one", paths)
	}
	content, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("read seed migration: %v", err)
	}

	seeded := make(map[string]map[string][]byte)
	total := 0
	for _, line := range strings.Split(string(content), "\n") {
		match := seedRowPattern.FindStringSubmatch(strings.TrimSpace(line))
		if match == nil {
			continue
		}
		group, name, value := match[1], match[2], match[3]
		total++
		if seeded[group] == nil {
			seeded[group] = make(map[string][]byte)
		}
		if _, exists := seeded[group][name]; exists {
			t.Fatalf("duplicate seed row for %s.%s", group, name)
		}
		seeded[group][name] = []byte(value)
	}

	want := 0
	for _, group := range builtinRegistry {
		for _, item := range group.Items {
			want++
			raw, ok := seeded[group.Name][item.Name]
			if !ok {
				t.Errorf("seed migration is missing %s.%s", group.Name, item.Name)
				continue
			}
			decoded, err := decodeTyped(item.Type, raw)
			if err != nil {
				t.Errorf("seed %s.%s does not decode as field type %d: %v", group.Name, item.Name, item.Type, err)
				continue
			}
			if decoded != item.Default {
				t.Errorf("seed %s.%s = %v, want registry default %v", group.Name, item.Name, decoded, item.Default)
			}
		}
	}
	if total != want {
		t.Fatalf("seed rows = %d, want %d registry items", total, want)
	}
}
