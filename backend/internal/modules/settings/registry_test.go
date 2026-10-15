package settings

import (
	"reflect"
	"sort"
	"testing"
)

func recipeGroups() []Group {
	return []Group{
		{
			Name: "demo",
			Items: []Definition{
				{Name: "title", Type: FieldTypeString, Default: "我的博客"},
				{Name: "port", Type: FieldTypeNumber, Default: int64(8080)},
				{Name: "maintenance", Type: FieldTypeBool, Default: false},
			},
		},
	}
}

func TestRegistryRecipeShape(t *testing.T) {
	reg, err := compileRegistry(recipeGroups())
	if err != nil {
		t.Fatalf("recipe-shaped registry rejected: %v", err)
	}
	def, ok := reg.groups["demo"]["port"]
	if !ok || def.Type != FieldTypeNumber || def.Default.(int64) != 8080 {
		t.Fatalf("recipe item lookup wrong: %+v ok=%v", def, ok)
	}
}

func TestBuiltinRegistryShape(t *testing.T) {
	reg, err := compileRegistry(builtinRegistry)
	if err != nil {
		t.Fatalf("builtin registry rejected: %v", err)
	}
	def, ok := reg.groups["register"]["enabled"]
	if !ok {
		t.Fatal("builtin registry missing register.enabled")
	}
	if def.Type != FieldTypeBool || def.Default.(bool) != true {
		t.Fatalf("register.enabled = %+v, want bool default true", def)
	}
	if !def.Public {
		t.Fatal("register.enabled must be Public, otherwise the frontend cannot read the signup gate")
	}

	siteItems, ok := reg.groups["site"]
	if !ok {
		t.Fatal("builtin registry missing the site group")
	}
	if len(siteItems) != 5 {
		t.Fatalf("site items = %d, want 5", len(siteItems))
	}
	for _, name := range []string{"title", "subtitle", "description", "keywords", "url"} {
		def, ok := siteItems[name]
		if !ok {
			t.Fatalf("builtin registry missing site.%s", name)
		}
		if def.Type != FieldTypeString || def.Default.(string) != "" {
			t.Fatalf("site.%s = %+v, want string default \"\"", name, def)
		}
		if !def.Public {
			t.Fatalf("site.%s must be Public, otherwise the frontend cannot read it", name)
		}
	}
}

func TestBuiltinPublicAllowlist(t *testing.T) {
	reg, err := compileRegistry(builtinRegistry)
	if err != nil {
		t.Fatalf("builtin registry rejected: %v", err)
	}

	var public []string
	for _, group := range reg.order {
		for name, def := range reg.groups[group] {
			if def.Public {
				public = append(public, group+"."+name)
			}
		}
	}
	sort.Strings(public)
	want := []string{
		"register.enabled",
		"site.description",
		"site.keywords",
		"site.subtitle",
		"site.title",
		"site.url",
	}
	if !reflect.DeepEqual(public, want) {
		t.Fatalf("public items = %v, want %v — marking a setting public exposes it anonymously", public, want)
	}
}

func TestRegistryValid(t *testing.T) {
	groups := []Group{
		{Name: "alpha", Items: []Definition{{Name: "enabled", Type: FieldTypeBool, Default: true}}},
		{Name: "beta", Items: []Definition{
			{Name: "label", Type: FieldTypeString, Default: "x"},
			{Name: "limit", Type: FieldTypeNumber, Default: int64(10)},
		}},
	}
	reg, err := compileRegistry(groups)
	if err != nil {
		t.Fatalf("valid registry rejected: %v", err)
	}
	if len(reg.order) != 2 || reg.order[0] != "alpha" || reg.order[1] != "beta" {
		t.Fatalf("order = %v, want [alpha beta]", reg.order)
	}
	if _, ok := reg.groups["beta"]["label"]; !ok {
		t.Fatal("beta.label missing")
	}
}

func TestRegistryEmpty(t *testing.T) {
	reg, err := compileRegistry(nil)
	if err != nil {
		t.Fatalf("empty registry rejected: %v", err)
	}
	if len(reg.order) != 0 || len(reg.groups) != 0 {
		t.Fatalf("empty registry not empty: %+v", reg)
	}
}

func TestRegistryRejects(t *testing.T) {
	cases := []struct {
		name   string
		groups []Group
	}{
		{"duplicate group", []Group{
			{Name: "site", Items: nil},
			{Name: "site", Items: nil},
		}},
		{"duplicate item", []Group{
			{Name: "site", Items: []Definition{
				{Name: "title", Type: FieldTypeString, Default: "a"},
				{Name: "title", Type: FieldTypeString, Default: "b"},
			}},
		}},
		{"group name uppercase", []Group{{Name: "Site", Items: nil}}},
		{"group name leading digit", []Group{{Name: "1a", Items: nil}}},
		{"group name dash", []Group{{Name: "a-b", Items: nil}}},
		{"item name uppercase", []Group{
			{Name: "site", Items: []Definition{{Name: "Title", Type: FieldTypeString, Default: "a"}}},
		}},
		{"item name leading digit", []Group{
			{Name: "site", Items: []Definition{{Name: "1a", Type: FieldTypeString, Default: "a"}}},
		}},
		{"item name dash", []Group{
			{Name: "site", Items: []Definition{{Name: "a-b", Type: FieldTypeString, Default: "a"}}},
		}},
		{"string default int64", []Group{
			{Name: "site", Items: []Definition{{Name: "title", Type: FieldTypeString, Default: int64(1)}}},
		}},
		{"number default float64", []Group{
			{Name: "site", Items: []Definition{{Name: "port", Type: FieldTypeNumber, Default: 8080.5}}},
		}},
		{"number default int", []Group{
			{Name: "site", Items: []Definition{{Name: "port", Type: FieldTypeNumber, Default: 8080}}},
		}},
		{"bool default string", []Group{
			{Name: "site", Items: []Definition{{Name: "flag", Type: FieldTypeBool, Default: "true"}}},
		}},
		{"unknown field type", []Group{
			{Name: "site", Items: []Definition{{Name: "x", Type: FieldType(9), Default: nil}}},
		}},
	}
	for _, tc := range cases {
		if _, err := compileRegistry(tc.groups); err == nil {
			t.Fatalf("%s: accepted, want error", tc.name)
		}
	}
}
