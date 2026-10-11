package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type UserPermission struct {
	ent.Schema
}

func (UserPermission) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.UUID("user_id", uuid.UUID{}),
		field.String("permission").NotEmpty(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (UserPermission) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("permissions").Unique().Required().Field("user_id"),
	}
}

func (UserPermission) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id", "permission").Unique(),
	}
}
