package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

type KeyPair struct {
	ent.Schema
}

func (KeyPair) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("openstack_name").Unique(),
		field.String("fingerprint"),
		field.String("public_key"),
		field.Enum("source_type").Values("user_uploaded", "system_generated"),
		field.Enum("sync_state").Values("synced", "missing", "error").Default("synced"),
		field.Time("last_synced_at"),
		field.Time("created_at").Immutable().Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (KeyPair) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("keypairs").Required(),
		edge.To("instances", Instance.Type),
	}
}
