package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"

	"website/backend/internal/database"
)

var (
	ErrUnknownGroup   = errors.New("settings: unknown group")
	ErrUnknownItem    = errors.New("settings: unknown item")
	ErrTypeMismatch   = errors.New("settings: type mismatch")
	ErrMethodMismatch = errors.New("settings: method/type misuse")
)

type ItemWrite struct {
	Name  string
	Type  FieldType
	Value any
}

type Service struct {
	repo     *repository
	registry Registry
	logger   *slog.Logger
}

func newServiceWith(db *database.Database, groups []Group, logger *slog.Logger) (*Service, error) {
	registry, err := compileRegistry(groups)
	if err != nil {
		return nil, err
	}
	var repo *repository
	if db != nil {
		repo = &repository{client: db.Client}
	}
	return &Service{repo: repo, registry: registry, logger: logger}, nil
}

func (s *Service) lookup(group, name string) (Definition, error) {
	items, ok := s.registry.groups[group]
	if !ok {
		return Definition{}, ErrUnknownGroup
	}
	def, ok := items[name]
	if !ok {
		return Definition{}, ErrUnknownItem
	}
	return def, nil
}

func (s *Service) getTyped[T any](ctx context.Context, group, name string, fieldType FieldType) (T, error) {
	var zero T
	def, err := s.lookup(group, name)
	if err != nil {
		return zero, err
	}
	if def.Type != fieldType {
		return zero, ErrMethodMismatch
	}
	raw, ok, err := s.repo.get(ctx, group, name)
	if err != nil {
		return zero, err
	}
	if !ok {
		return def.Default.(T), nil
	}
	value, err := decodeTyped(fieldType, raw)
	if err != nil {
		s.fallback(group, name)
		return def.Default.(T), nil
	}
	return value.(T), nil
}

func (s *Service) GetString(ctx context.Context, group, name string) (string, error) {
	return s.getTyped[string](ctx, group, name, FieldTypeString)
}

func (s *Service) GetNumber(ctx context.Context, group, name string) (int64, error) {
	return s.getTyped[int64](ctx, group, name, FieldTypeNumber)
}

func (s *Service) GetBool(ctx context.Context, group, name string) (bool, error) {
	return s.getTyped[bool](ctx, group, name, FieldTypeBool)
}

type GroupValues struct {
	Group  string
	Values map[string]any
}

func (s *Service) GetGroup(ctx context.Context, group string) (map[string]any, error) {
	return s.readGroup(ctx, group, func(Definition) bool { return true })
}

func (s *Service) PublicSettings(ctx context.Context) ([]GroupValues, error) {
	out := make([]GroupValues, 0, len(s.registry.order))
	for _, group := range s.registry.order {
		values, err := s.readGroup(ctx, group, func(def Definition) bool { return def.Public })
		if err != nil {
			return nil, err
		}
		if len(values) == 0 {
			continue
		}
		out = append(out, GroupValues{Group: group, Values: values})
	}
	return out, nil
}

func (s *Service) readGroup(ctx context.Context, group string, keep func(Definition) bool) (map[string]any, error) {
	items, ok := s.registry.groups[group]
	if !ok {
		return nil, ErrUnknownGroup
	}
	rows, err := s.repo.listByGroup(ctx, group)
	if err != nil {
		return nil, err
	}
	overrides := make(map[string]json.RawMessage, len(rows))
	for _, row := range rows {
		overrides[row.Name] = row.Value
	}
	out := make(map[string]any, len(items))
	for name, def := range items {
		if !keep(def) {
			continue
		}
		raw, ok := overrides[name]
		if !ok {
			out[name] = def.Default
			continue
		}
		value, err := decodeTyped(def.Type, raw)
		if err != nil {
			s.fallback(group, name)
			out[name] = def.Default
			continue
		}
		out[name] = value
	}
	return out, nil
}

func (s *Service) SetString(ctx context.Context, group, name, value string) error {
	def, err := s.lookup(group, name)
	if err != nil {
		return err
	}
	if def.Type != FieldTypeString {
		return ErrMethodMismatch
	}
	return s.repo.upsert(ctx, group, name, marshalJSON(value))
}

func (s *Service) SetNumber(ctx context.Context, group, name string, value int64) error {
	def, err := s.lookup(group, name)
	if err != nil {
		return err
	}
	if def.Type != FieldTypeNumber {
		return ErrMethodMismatch
	}
	return s.repo.upsert(ctx, group, name, marshalJSON(value))
}

func (s *Service) SetBool(ctx context.Context, group, name string, value bool) error {
	def, err := s.lookup(group, name)
	if err != nil {
		return err
	}
	if def.Type != FieldTypeBool {
		return ErrMethodMismatch
	}
	return s.repo.upsert(ctx, group, name, marshalJSON(value))
}

func (s *Service) SetGroup(ctx context.Context, group string, items []ItemWrite) error {
	values, err := s.resolveGroupWrites(group, items)
	if err != nil {
		return err
	}
	return s.repo.upsertMany(ctx, group, values)
}

type ItemError struct {
	Item string
	Err  error
}

func (e *ItemError) Error() string { return e.Err.Error() }

func (e *ItemError) Unwrap() error { return e.Err }

func (s *Service) PatchGroup(ctx context.Context, group string, raw map[string]json.RawMessage) (map[string]any, error) {
	items, ok := s.registry.groups[group]
	if !ok {
		return nil, ErrUnknownGroup
	}
	if len(raw) == 0 {
		return nil, ErrUnknownItem
	}

	names := make([]string, 0, len(raw))
	for name := range raw {
		names = append(names, name)
	}
	sort.Strings(names)

	writes := make([]ItemWrite, 0, len(names))
	for _, name := range names {
		def, ok := items[name]
		if !ok {
			return nil, &ItemError{Item: name, Err: ErrUnknownItem}
		}
		value, err := decodeTyped(def.Type, raw[name])
		if err != nil {
			return nil, &ItemError{Item: name, Err: ErrTypeMismatch}
		}
		writes = append(writes, ItemWrite{Name: name, Type: def.Type, Value: value})
	}

	values, err := s.resolveGroupWrites(group, writes)
	if err != nil {
		return nil, err
	}
	if validator, ok := s.registry.validators[group]; ok && validatorApplies(validator, writes) {
		merged, err := s.mergedGroupValues(ctx, group, writes)
		if err != nil {
			return nil, err
		}
		if err := validator.check(merged); err != nil {
			return nil, err
		}
	}
	if err := s.repo.upsertMany(ctx, group, values); err != nil {
		return nil, err
	}
	return s.GetGroup(ctx, group)
}

func (s *Service) resolveGroupWrites(group string, items []ItemWrite) (map[string]json.RawMessage, error) {
	if _, ok := s.registry.groups[group]; !ok {
		return nil, ErrUnknownGroup
	}
	if len(items) == 0 {
		return nil, ErrUnknownItem
	}
	values := make(map[string]json.RawMessage, len(items))
	for _, item := range items {
		def, ok := s.registry.groups[group][item.Name]
		if !ok {
			return nil, ErrUnknownItem
		}
		if item.Type != def.Type {
			return nil, ErrTypeMismatch
		}
		switch item.Type {
		case FieldTypeString:
			if _, ok := item.Value.(string); !ok {
				return nil, ErrTypeMismatch
			}
		case FieldTypeNumber:
			if _, ok := item.Value.(int64); !ok {
				return nil, ErrTypeMismatch
			}
		case FieldTypeBool:
			if _, ok := item.Value.(bool); !ok {
				return nil, ErrTypeMismatch
			}
		default:
			return nil, ErrTypeMismatch
		}
		values[item.Name] = marshalJSON(item.Value)
	}
	return values, nil
}

func validatorApplies(validator groupValidator, writes []ItemWrite) bool {
	for _, write := range writes {
		for _, item := range validator.items {
			if write.Name == item {
				return true
			}
		}
	}
	return false
}

func (s *Service) mergedGroupValues(ctx context.Context, group string, items []ItemWrite) (map[string]any, error) {
	merged, err := s.readGroup(ctx, group, func(Definition) bool { return true })
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		merged[item.Name] = item.Value
	}
	return merged, nil
}

func (s *Service) ResetItem(ctx context.Context, group, name string) error {
	if _, err := s.lookup(group, name); err != nil {
		return err
	}
	_, err := s.repo.delete(ctx, group, name)
	return err
}

func (s *Service) fallback(group, name string) {
	s.logger.Warn("settings: bad row, using default", "group", group, "name", name, "category", "type_mismatch")
}

func decodeTyped(fieldType FieldType, raw json.RawMessage) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var out any
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	switch fieldType {
	case FieldTypeString:
		s, ok := out.(string)
		if !ok {
			return nil, fmt.Errorf("not a JSON string")
		}
		return s, nil
	case FieldTypeNumber:
		number, ok := out.(json.Number)
		if !ok {
			return nil, fmt.Errorf("not a JSON number")
		}
		return strconv.ParseInt(string(number), 10, 64)
	case FieldTypeBool:
		b, ok := out.(bool)
		if !ok {
			return nil, fmt.Errorf("not a JSON bool")
		}
		return b, nil
	}
	return nil, fmt.Errorf("unknown field type %d", fieldType)
}

func decodeNumber(raw json.RawMessage) (int64, error) {
	value, err := decodeTyped(FieldTypeNumber, raw)
	if err != nil {
		return 0, err
	}
	return value.(int64), nil
}

func marshalJSON(value any) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}
