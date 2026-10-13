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
	Name  string
	Items []Definition
}

type Registry struct {
	groups map[string]map[string]Definition
	order  []string
}
