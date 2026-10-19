package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"

	"website/backend/internal/common/rbac"
)

type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }),
		field.String("email").NotEmpty().Unique(),
		field.String("nickname").NotEmpty().Unique(),
		field.String("bio").Optional().Nillable(),
		field.String("password_hash").NotEmpty(),
		field.Int("auth_version").Default(0),
		field.UUID("group_id", uuid.UUID{}).Default(func() uuid.UUID { return rbac.GroupIDUser }),
		field.String("totp_secret").Optional().Nillable(),
		field.Time("totp_enabled_at").Optional().Nillable(),
		field.Int64("totp_last_counter").Optional().Nillable(),
		field.Time("totp_setup_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("group", Group.Type).Ref("users").Unique().Required().Field("group_id"),
		edge.To("permissions", UserPermission.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
		edge.To("recoveryCodes", RecoveryCode.Type).Annotations(entsql.OnDelete(entsql.Cascade)),
	}
}

func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("group_id"),
	}
}
