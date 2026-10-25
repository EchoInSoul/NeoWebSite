package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type CaptchaChallenge struct {
	ent.Schema
}

func (CaptchaChallenge) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("scope").MaxLen(16).NotEmpty(),
		field.Int("answer_x"),
		field.Int("answer_y"),
		field.Int("status").Default(0),
		field.String("token_hash").MaxLen(64).Optional().Nillable().Unique(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("expires_at"),
	}
}

func (CaptchaChallenge) Indexes() []ent.Index {
	return []ent.Index{index.Fields("expires_at")}
}
