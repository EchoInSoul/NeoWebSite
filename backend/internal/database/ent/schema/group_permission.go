package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type GroupPermission struct {
	ent.Schema
}

func (GroupPermission) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.UUID("group_id", uuid.UUID{}),
		field.String("permission").NotEmpty(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (GroupPermission) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", Group.Type).Ref("permissions").Unique().Required().Field("group_id"),
	}
}

func (GroupPermission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("group_id", "permission").Unique(),
	}
}
