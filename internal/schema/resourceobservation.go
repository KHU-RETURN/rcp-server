package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
	"time"
)

type ResourceObservation struct{ ent.Schema }

func (ResourceObservation) Fields() []ent.Field {
	return []ent.Field{field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("resource_key").Unique(),
		field.String("kind"),
		field.String("resource_id"),
		field.String("status"),
		field.String("consistency"),
		field.String("reason").Default(""),
		field.Time("observed_at").Default(time.Now)}
}
