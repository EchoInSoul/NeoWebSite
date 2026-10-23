package settings

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

var seedRowPattern = regexp.MustCompile(`^\(gen_random_uuid\(\), '([a-z0-9_]+)', '([a-z0-9_]+)', '(.*)'::jsonb, now\(\)\)[,;]?$`)

func TestSeedMigrationMatchesBuiltinRegistry(t *testing.T) {
	paths, err := filepath.Glob("../../../migrations/*_seed_*.sql")
	if err != nil {
		t.Fatalf("glob seed migrations: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no seed migrations found")
	}

	sort.Strings(paths)
	seeded := make(map[string]map[string][]byte)
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read seed migration %s: %v", path, err)
		}
		seen := make(map[string]bool)
		for _, line := range strings.Split(string(content), "\n") {
			match := seedRowPattern.FindStringSubmatch(strings.TrimSpace(line))
			if match == nil {
				continue
			}
			group, name, value := match[1], match[2], match[3]
			key := group + "." + name
			if seen[key] {
				t.Fatalf("duplicate seed row for %s in %s", key, path)
			}
			seen[key] = true
			if seeded[group] == nil {
				seeded[group] = make(map[string][]byte)
			}
			seeded[group][name] = []byte(value)
		}
	}

	total := 0
	for _, items := range seeded {
		total += len(items)
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
		t.Fatalf("seeded items = %d, want %d registry items", total, want)
	}
}
