package apiresponse

import "encoding/json"

type ItemList[T any] struct {
	Items []T `json:"items"`
}

func (l ItemList[T]) MarshalJSON() ([]byte, error) {
	items := l.Items
	if items == nil {
		items = []T{}
	}
	return json.Marshal(struct {
		Items []T `json:"items"`
	}{Items: items})
}

type Deleted struct {
	ID      string `json:"id"`
	Deleted bool   `json:"deleted"`
}

type DeletedBatch struct {
	IDs     []string `json:"ids"`
	Deleted bool     `json:"deleted"`
}

func (d DeletedBatch) MarshalJSON() ([]byte, error) {
	ids := d.IDs
	if ids == nil {
		ids = []string{}
	}
	return json.Marshal(struct {
		IDs     []string `json:"ids"`
		Deleted bool     `json:"deleted"`
	}{IDs: ids, Deleted: d.Deleted})
}

type Empty struct{}
