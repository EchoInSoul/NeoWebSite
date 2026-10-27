package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type MediaAsset struct {
	ent.Schema
}

func (MediaAsset) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("provider").NotEmpty(),
		field.String("key").NotEmpty().Unique(),
		field.Int64("size_bytes").NonNegative(),
		field.String("original_name"),
		field.UUID("uploaded_by", uuid.UUID{}).Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (MediaAsset) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("uploader", User.Type).Ref("mediaAssets").Unique().Field("uploaded_by"),
	}
}

func (MediaAsset) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("uploaded_by"),
	}
}
