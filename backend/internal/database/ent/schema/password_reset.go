package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PasswordReset struct {
	ent.Schema
}

func (PasswordReset) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.UUID("user_id", uuid.UUID{}),
		field.String("token_hash").NotEmpty().Unique(),
		field.Time("expires_at"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (PasswordReset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("passwordResets").Unique().Required().Field("user_id"),
	}
}

func (PasswordReset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("user_id").Unique(),
		index.Fields("expires_at"),
	}
}
