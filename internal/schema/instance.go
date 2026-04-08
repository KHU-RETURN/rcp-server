package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type Instance struct {
	ent.Schema
}

func (Instance) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("openstack_id").Unique(),
		field.String("name"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (Instance) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("instances").Unique().Required(),
	}
}
