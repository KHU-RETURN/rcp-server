package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type KeyPair struct {
	ent.Schema
}

func (KeyPair) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name"),
		field.Time("created_at").Default(time.Now).Immutable(),
	}
}

func (KeyPair) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("user", User.Type).Ref("keypairs").Unique().Required(),
	}
}

func (KeyPair) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("name").Edges("user").Unique(),
	}
}
