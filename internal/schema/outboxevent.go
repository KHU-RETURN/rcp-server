package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
	"time"
)

type OutboxEvent struct{ ent.Schema }

func (OutboxEvent) Fields() []ent.Field {
	return []ent.Field{field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("operation_id", uuid.UUID{}),
		field.String("kind"),
		field.String("payload").Default(""),
		field.Int("attempts").Default(0),
		field.Time("next_attempt_at").Default(time.Now),
		field.Time("lease_until").Default(func() time.Time { return time.Unix(0, 0) }),
		field.String("lease_token").Default(""),
		field.Time("processed_at").Optional().Nillable(),
		field.String("last_error").Default(""),
		field.Time("created_at").Default(time.Now).Immutable()}
}

func (OutboxEvent) Indexes() []ent.Index {
	return []ent.Index{index.Fields("processed_at", "next_attempt_at")}
}
