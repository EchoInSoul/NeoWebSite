package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type RecoveryCode struct {
	ent.Schema
}

func (RecoveryCode) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.UUID("user_id", uuid.UUID{}),
		field.String("code_hash").NotEmpty(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("used_at").Optional().Nillable(),
	}
}

func (RecoveryCode) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("recoveryCodes").Unique().Required().Field("user_id"),
	}
}

func (RecoveryCode) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id"),
	}
}
