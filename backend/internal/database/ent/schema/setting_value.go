package schema

import (
	"encoding/json"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type SettingValue struct {
	ent.Schema
}

func (SettingValue) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("setting_group").NotEmpty(),
		field.String("name").NotEmpty(),
		field.JSON("value", json.RawMessage{}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (SettingValue) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("setting_group", "name").Unique(),
	}
}
