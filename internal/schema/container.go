package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

type Container struct {
	ent.Schema
}

func (Container) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(uuid.New),
		field.UUID("openstack_name", uuid.UUID{}).Unique(),
		field.String("name"),
		field.Time("created_at").Immutable().Default(time.Now),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
	}
}

func (Container) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).Ref("containers").Required().Unique(),
	}
}

func (Container) Indexes() []ent.Index {
	return []ent.Index{
		// 같은 소유자가 동일한 이름의 컨테이너를 동시에 생성해도 중복 행이 생기지 않도록 보장한다.
		index.Fields("name").Edges("owner").Unique(),
	}
}
