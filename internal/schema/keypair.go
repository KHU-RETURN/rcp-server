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
		field.Time("created_at").Immutable().Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (KeyPair) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("keypairs").Required().Unique(),
		edge.To("instances", Instance.Type),
	}
}
