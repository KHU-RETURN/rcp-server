package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"time"
)

type ResourceOperation struct{ ent.Schema }

func (ResourceOperation) Fields() []ent.Field {
	return []ent.Field{field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("owner_id", uuid.UUID{}),
		field.String("request_key"),
		field.String("fingerprint"),
		field.String("kind"),
		field.String("resource_id").Default(""),
		field.String("payload"),
		field.String("status").Default("PENDING"),
		field.Bool("dispatched").Default(false),
		field.String("active_key").Optional().Nillable().Unique(),
		field.String("last_error").Default(""),
		field.Time("created_at").Default(time.Now).Immutable(),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now)}
}

func (ResourceOperation) Indexes() []ent.Index {
	return []ent.Index{index.Fields("owner_id", "request_key").Unique()}
}
