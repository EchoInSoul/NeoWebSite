package settings

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
)

func sampleGroups() []Group {
	return []Group{
		{
			Name: "site",
			Items: []Definition{
				{Name: "title", Type: FieldTypeString, Default: "我的博客"},
				{Name: "port", Type: FieldTypeNumber, Default: int64(8080)},
				{Name: "maintenance", Type: FieldTypeBool, Default: false},
			},
		},
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newUnitService(t *testing.T) *Service {
	t.Helper()
	svc, err := newServiceWith(nil, sampleGroups(), testLogger())
	if err != nil {
		t.Fatalf("newServiceWith: %v", err)
	}
	return svc
}

func TestServiceMethodMisuse(t *testing.T) {
	svc := newUnitService(t)
	ctx := context.Background()

	if _, err := svc.GetString(ctx, "site", "port"); !errors.Is(err, ErrMethodMismatch) {
		t.Fatalf("GetString on number item = %v, want ErrMethodMismatch", err)
	}
	if _, err := svc.GetNumber(ctx, "site", "title"); !errors.Is(err, ErrMethodMismatch) {
		t.Fatalf("GetNumber on string item = %v, want ErrMethodMismatch", err)
	}
	if _, err := svc.GetNumber(ctx, "site", "maintenance"); !errors.Is(err, ErrMethodMismatch) {
		t.Fatalf("GetNumber on bool item = %v, want ErrMethodMismatch", err)
	}
	if _, err := svc.GetBool(ctx, "site", "title"); !errors.Is(err, ErrMethodMismatch) {
		t.Fatalf("GetBool on string item = %v, want ErrMethodMismatch", err)
	}
	if err := svc.SetString(ctx, "site", "port", "x"); !errors.Is(err, ErrMethodMismatch) {
		t.Fatalf("SetString on number item = %v, want ErrMethodMismatch", err)
	}
	if err := svc.SetNumber(ctx, "site", "title", 1); !errors.Is(err, ErrMethodMismatch) {
		t.Fatalf("SetNumber on string item = %v, want ErrMethodMismatch", err)
	}
	if err := svc.SetBool(ctx, "site", "title", true); !errors.Is(err, ErrMethodMismatch) {
		t.Fatalf("SetBool on string item = %v, want ErrMethodMismatch", err)
	}
}

func TestServiceUnknown(t *testing.T) {
	svc := newUnitService(t)
	ctx := context.Background()

	if _, err := svc.GetString(ctx, "nope", "title"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("GetString unknown group = %v, want ErrUnknownGroup", err)
	}
	if err := svc.SetString(ctx, "nope", "title", "x"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("SetString unknown group = %v, want ErrUnknownGroup", err)
	}
	if err := svc.ResetItem(ctx, "nope", "title"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("ResetItem unknown group = %v, want ErrUnknownGroup", err)
	}
	if _, err := svc.GetGroup(ctx, "nope"); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("GetGroup unknown group = %v, want ErrUnknownGroup", err)
	}
	if _, err := svc.GetString(ctx, "site", "ghost"); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("GetString unknown item = %v, want ErrUnknownItem", err)
	}
	if err := svc.SetString(ctx, "site", "ghost", "x"); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("SetString unknown item = %v, want ErrUnknownItem", err)
	}
	if err := svc.ResetItem(ctx, "site", "ghost"); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("ResetItem unknown item = %v, want ErrUnknownItem", err)
	}
}

func TestDecodeNumberBoundary(t *testing.T) {
	rejected := []json.RawMessage{
		json.RawMessage(`3.5`),
		json.RawMessage(`1e3`),
		json.RawMessage(`"3"`),
		json.RawMessage(`9223372036854775808`),
		json.RawMessage(`true`),
		json.RawMessage(`[3]`),
	}
	for _, raw := range rejected {
		if _, err := decodeNumber(raw); err == nil {
			t.Fatalf("decodeNumber(%s) accepted, want error", raw)
		}
	}
	accepted := map[string]int64{
		`3`:                     3,
		`-5`:                    -5,
		`0`:                     0,
		`9223372036854775807`:   9223372036854775807,
		`-9223372036854775808`:  -9223372036854775808,
	}
	for raw, want := range accepted {
		got, err := decodeNumber(json.RawMessage(raw))
		if err != nil || got != want {
			t.Fatalf("decodeNumber(%s) = (%d, %v), want %d", raw, got, err, want)
		}
	}
}

func TestServiceItemWriteTypeMismatch(t *testing.T) {
	svc := newUnitService(t)
	ctx := context.Background()

	cases := []ItemWrite{
		{Name: "port", Type: FieldTypeNumber, Value: 3.5},
		{Name: "port", Type: FieldTypeNumber, Value: int32(3)},
		{Name: "port", Type: FieldTypeNumber, Value: "3"},
		{Name: "title", Type: FieldTypeString, Value: int64(1)},
		{Name: "maintenance", Type: FieldTypeBool, Value: "true"},
		{Name: "port", Type: FieldTypeString, Value: "x"},
	}
	for _, item := range cases {
		if err := svc.SetGroup(ctx, "site", []ItemWrite{item}); !errors.Is(err, ErrTypeMismatch) {
			t.Fatalf("SetGroup %+v = %v, want ErrTypeMismatch", item, err)
		}
	}

	good := ItemWrite{Name: "title", Type: FieldTypeString, Value: "ok"}
	if err := svc.SetGroup(ctx, "site", []ItemWrite{good, {Name: "ghost", Type: FieldTypeString, Value: "x"}}); !errors.Is(err, ErrUnknownItem) {
		t.Fatalf("SetGroup with unknown item = %v, want ErrUnknownItem", err)
	}
	if err := svc.SetGroup(ctx, "site", nil); err == nil {
		t.Fatal("SetGroup with no items accepted, want error")
	}
	if err := svc.SetGroup(ctx, "nope", []ItemWrite{good}); !errors.Is(err, ErrUnknownGroup) {
		t.Fatalf("SetGroup unknown group = %v, want ErrUnknownGroup", err)
	}
}
