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
		field.String("status"),
		field.String("image_id"),
		field.String("flavor_id"),
		field.String("fixed_ip").Optional().Nillable(),
		field.String("floating_ip").Optional().Nillable(),
		field.Time("provider_created_at"),
		field.Enum("sync_state").Values("synced", "missing", "error").Default("synced"),
		field.Time("last_synced_at"),
		field.Time("created_at").Immutable().Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Instance) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("instances").Required(),
		edge.From("keypair", KeyPair.Type).Ref("instances").Unique(),
	}
}
