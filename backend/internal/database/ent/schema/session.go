package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

type Session struct {
	ent.Schema
}

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.String("token").NotEmpty().Unique(),
		field.Bytes("data").NotEmpty(),
		field.Time("expiry"),
	}
}

func (Session) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("expiry"),
	}
}
