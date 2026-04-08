package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Session struct {
	ent.Schema
}

func (Session) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("access_token"),
		field.String("refresh_token"),
		field.Time("expiry"),
		field.Enum("provider").Values("GOOGLE"),
		field.String("provider_token").Sensitive(),
		field.String("provider_refresh").Optional().Nillable().Sensitive(),
		field.Time("provider_expiry").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Session) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("sessions").Unique().Required(),
	}
}
