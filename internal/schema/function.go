package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// Function stores a WASI command owned by an RCP user.
type Function struct{ ent.Schema }

func (Function) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.String("name").NotEmpty(),
		field.Bytes("wasm").NotEmpty().Sensitive(),
		field.String("language").Default("wasm"),
		field.Bool("data_mode").Default(false),
		field.Bytes("source").Optional().Sensitive(),
		field.Bytes("key_hash").Optional().Sensitive(),
		field.Time("key_created_at").Optional().Nillable(),
		field.Time("key_expires_at").Optional().Nillable(),
		field.Time("created_at").Immutable().Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Function) Edges() []ent.Edge {
	return []ent.Edge{edge.From("owner", User.Type).Ref("functions").Required().Unique()}
}

func (Function) Indexes() []ent.Index {
	return []ent.Index{index.Fields("name").Edges("owner").Unique()}
}
