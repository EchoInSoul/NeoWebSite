package settings

type FieldType int

const (
	FieldTypeString FieldType = iota + 1
	FieldTypeNumber
	FieldTypeBool
)

type Definition struct {
	Name    string
	Type    FieldType
	Default any
	Public  bool
}

type Group struct {
	Name       string
	Items      []Definition
	ValidateOn []string
	Validate   func(values map[string]any, touched map[string]bool) error
}

type groupValidator struct {
	items []string
	check func(values map[string]any, touched map[string]bool) error
}

type Registry struct {
	groups     map[string]map[string]Definition
	validators map[string]groupValidator
	order      []string
}
