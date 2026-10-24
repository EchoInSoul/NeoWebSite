package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type PendingRegistration struct {
	ent.Schema
}

func (PendingRegistration) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("email").NotEmpty().Unique(),
		field.String("nickname").NotEmpty(),
		field.String("password_hash").NotEmpty(),
		field.String("token_hash").NotEmpty().Unique(),
		field.Time("expires_at"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (PendingRegistration) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expires_at"),
	}
}
