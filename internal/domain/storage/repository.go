package storage

import (
	"context"

	"github.com/google/uuid"

	"github.com/KHU-RETURN/rcp-server/ent"
	entcontainer "github.com/KHU-RETURN/rcp-server/ent/container"
	entuser "github.com/KHU-RETURN/rcp-server/ent/user"
)

type Repository struct {
	client *ent.Client
}

func NewRepository(client *ent.Client) *Repository {
	return &Repository{client: client}
}

func (r *Repository) Save(ctx context.Context, ownerID uuid.UUID, c *Container) error {
	row, err := r.client.Container.Create().
		SetOwnerID(ownerID).
		SetOpenstackName(c.OpenstackName).
		SetName(c.Name).
		Save(ctx)
	if err != nil {
		// (owner, name) 유니크 인덱스 위반은 동시 생성 경쟁에서 이미 같은 이름이
		// 저장됐다는 뜻이므로 도메인 에러로 변환한다.
		if ent.IsConstraintError(err) {
			return ErrContainerAlreadyExists
		}
		return err
	}
	c.CreatedAt = row.CreatedAt
	return nil
}

func (r *Repository) FindByName(ctx context.Context, ownerID uuid.UUID, name string) (*Container, error) {
	row, err := r.findByNameQuery(ownerID, name).Only(ctx)
	if err != nil {
		switch {
		case ent.IsNotFound(err):
			return nil, nil
		case ent.IsNotSingular(err):
			// 유니크 인덱스 도입 이전에 생성된 중복 행이 남아 있어도
			// 조회가 실패하지 않도록 첫 행으로 폴백한다.
			row, err = r.findByNameQuery(ownerID, name).First(ctx)
			if err != nil {
				return nil, err
			}
		default:
			return nil, err
		}
	}
	c := entToContainer(row)
	return &c, nil
}

func (r *Repository) findByNameQuery(ownerID uuid.UUID, name string) *ent.ContainerQuery {
	return r.client.Container.Query().
		Where(
			entcontainer.Name(name),
			entcontainer.HasOwnerWith(entuser.ID(ownerID)),
		)
}

func (r *Repository) ListByOwner(ctx context.Context, ownerID uuid.UUID) ([]Container, error) {
	rows, err := r.client.Container.Query().
		Where(entcontainer.HasOwnerWith(entuser.ID(ownerID))).
		All(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Container, len(rows))
	for i, row := range rows {
		result[i] = entToContainer(row)
	}
	return result, nil
}

func (r *Repository) Delete(ctx context.Context, ownerID uuid.UUID, name string) error {
	_, err := r.client.Container.Delete().
		Where(
			entcontainer.Name(name),
			entcontainer.HasOwnerWith(entuser.ID(ownerID)),
		).Exec(ctx)
	return err
}

func entToContainer(row *ent.Container) Container {
	return Container{
		OpenstackName: row.OpenstackName,
		Name:          row.Name,
		CreatedAt:     row.CreatedAt,
	}
}
