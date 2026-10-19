package apiresponse

import (
	"encoding/json"
	"testing"
)

func marshal(t *testing.T, value any) string {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

func TestItemListMarshalsItems(t *testing.T) {
	if got := marshal(t, ItemList[int]{Items: []int{1, 2}}); got != `{"items":[1,2]}` {
		t.Fatalf("json = %s, want {\"items\":[1,2]}", got)
	}
}

func TestItemListMarshalsNilAsEmptyArray(t *testing.T) {
	if got := marshal(t, ItemList[string]{}); got != `{"items":[]}` {
		t.Fatalf("json = %s, want {\"items\":[]}", got)
	}
}

func TestDeletedMarshalsConfirmation(t *testing.T) {
	if got := marshal(t, Deleted{ID: "abc", Deleted: true}); got != `{"id":"abc","deleted":true}` {
		t.Fatalf("json = %s, want {\"id\":\"abc\",\"deleted\":true}", got)
	}
}

func TestDeletedBatchMarshalsIDs(t *testing.T) {
	if got := marshal(t, DeletedBatch{IDs: []string{"a", "b"}, Deleted: true}); got != `{"ids":["a","b"],"deleted":true}` {
		t.Fatalf("json = %s, want {\"ids\":[\"a\",\"b\"],\"deleted\":true}", got)
	}
}

func TestDeletedBatchMarshalsNilAsEmptyArray(t *testing.T) {
	if got := marshal(t, DeletedBatch{}); got != `{"ids":[],"deleted":false}` {
		t.Fatalf("json = %s, want {\"ids\":[],\"deleted\":false}", got)
	}
}

func TestEmptyMarshalsEmptyObject(t *testing.T) {
	if got := marshal(t, Empty{}); got != `{}` {
		t.Fatalf("json = %s, want {}", got)
	}
}
